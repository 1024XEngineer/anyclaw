package enterprise

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"sync"
)

// CryptoManager handles encryption at rest with key derivation and rotation.
type CryptoManager struct {
	mu           sync.RWMutex
	dataKey      []byte
	keyEncrypted []byte
	wrapKey      *rsa.PrivateKey
}

// NewCryptoManager creates a new crypto manager. If wrapKeyPath is provided,
// it uses envelope encryption with RSA key wrapping.
func NewCryptoManager(dataKey []byte, wrapKeyPath string) (*CryptoManager, error) {
	cm := &CryptoManager{
		dataKey: dataKey,
	}

	if wrapKeyPath != "" {
		if err := cm.loadWrapKey(wrapKeyPath); err != nil {
			return nil, fmt.Errorf("failed to load wrap key: %w", err)
		}
		cm.keyEncrypted = cm.wrapDataKey(dataKey)
	}

	return cm, nil
}

func (cm *CryptoManager) loadWrapKey(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("key is not RSA")
	}

	cm.wrapKey = rsaKey
	return nil
}

func (cm *CryptoManager) wrapDataKey(dataKey []byte) []byte {
	if cm.wrapKey == nil {
		return nil
	}

	ciphertext, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &cm.wrapKey.PublicKey, dataKey, nil)
	return ciphertext
}

// Encrypt encrypts plaintext using AES-256-GCM.
func (cm *CryptoManager) Encrypt(plaintext []byte) (string, error) {
	cm.mu.RLock()
	key := cm.dataKey
	cm.mu.RUnlock()

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext encrypted with Encrypt.
func (cm *CryptoManager) Decrypt(ciphertextB64 string) ([]byte, error) {
	cm.mu.RLock()
	key := cm.dataKey
	cm.mu.RUnlock()

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptString encrypts a string value.
func (cm *CryptoManager) EncryptString(plaintext string) (string, error) {
	return cm.Encrypt([]byte(plaintext))
}

// DecryptString decrypts a string value.
func (cm *CryptoManager) DecryptString(ciphertextB64 string) (string, error) {
	plaintext, err := cm.Decrypt(ciphertextB64)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// RotateDataKey generates a new data key and re-encrypts it with the wrap key.
func (cm *CryptoManager) RotateDataKey() error {
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}

	cm.mu.Lock()
	cm.dataKey = newKey
	if cm.wrapKey != nil {
		cm.keyEncrypted = cm.wrapDataKey(newKey)
	}
	cm.mu.Unlock()

	return nil
}

// GenerateRSAKeyPair generates an RSA key pair and saves it to files.
func GenerateRSAKeyPair(privateKeyPath, publicKeyPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	privBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	}

	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(privBlock), 0o600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}

	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(pubBlock), 0o644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}
