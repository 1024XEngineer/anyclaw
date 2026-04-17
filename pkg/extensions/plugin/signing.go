package plugin

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SignerType string

const (
	SignerTypeGPG     SignerType = "gpg"
	SignerTypeRSA     SignerType = "rsa"
	SignerTypeEd25519 SignerType = "ed25519"
)

type Signature struct {
	Signer    string     `json:"signer"`
	Type      SignerType `json:"type"`
	Algorithm string     `json:"algorithm"`
	Value     string     `json:"value"`
	Timestamp time.Time  `json:"timestamp"`
	KeyID     string     `json:"key_id,omitempty"`
}

type KeyPair struct {
	Type       SignerType `json:"type"`
	PublicKey  string     `json:"public_key"`
	PrivateKey string     `json:"private_key,omitempty"`
	KeyID      string     `json:"key_id"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type SignatureVerifier struct {
	trustedKeys map[string]*KeyPair
}

func GenerateKeyPair(keyType SignerType, bits int) (*KeyPair, error) {
	switch keyType {
	case SignerTypeRSA:
		return generateRSAKeyPair(bits)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", keyType)
	}
}

func generateRSAKeyPair(bits int) (*KeyPair, error) {
	if bits < 2048 {
		bits = 2048
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	})

	keyID := fmt.Sprintf("%x", sha256.Sum256(pubKeyBytes))[:16]

	return &KeyPair{
		Type:       SignerTypeRSA,
		PublicKey:  string(pubKeyPEM),
		PrivateKey: string(privKeyPEM),
		KeyID:      keyID,
		CreatedAt:  time.Now(),
	}, nil
}

func SignManifest(manifest *Manifest, keyPair *KeyPair) (*Signature, error) {
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	hash := sha256.Sum256(manifestBytes)

	var signature []byte
	switch keyPair.Type {
	case SignerTypeRSA:
		block, _ := pem.Decode([]byte(keyPair.PrivateKey))
		if block == nil {
			return nil, fmt.Errorf("failed to decode private key")
		}
		privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		rsaKey := privKey.(*rsa.PrivateKey)
		signature, err = rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hash[:])
		if err != nil {
			return nil, fmt.Errorf("failed to sign: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported key type: %s", keyPair.Type)
	}

	return &Signature{
		Signer:    manifest.Signer,
		Type:      keyPair.Type,
		Algorithm: "SHA256-RSA",
		Value:     base64.StdEncoding.EncodeToString(signature),
		Timestamp: time.Now(),
		KeyID:     keyPair.KeyID,
	}, nil
}

func VerifySignature(manifest *Manifest, signature *Signature, publicKey string) (bool, error) {
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return false, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	hash := sha256.Sum256(manifestBytes)

	sigBytes, err := base64.StdEncoding.DecodeString(signature.Value)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return false, fmt.Errorf("failed to decode public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return false, fmt.Errorf("not an RSA public key")
	}

	err = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sigBytes)
	if err != nil {
		return false, fmt.Errorf("signature verification failed: %w", err)
	}

	return true, nil
}

func loadManifestFromFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func SignPluginDir(dir string, keyPair *KeyPair) (*Signature, error) {
	manifest, err := loadManifestFromFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	return SignManifest(manifest, keyPair)
}

func VerifyPluginDir(dir string, signature *Signature, publicKey string) (bool, error) {
	manifest, err := loadManifestFromFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return false, fmt.Errorf("failed to load manifest: %w", err)
	}

	return VerifySignature(manifest, signature, publicKey)
}

func SaveSignature(dir string, signature *Signature) error {
	data, err := json.MarshalIndent(signature, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plugin.signature"), data, 0644)
}

func LoadSignature(dir string) (*Signature, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.signature"))
	if err != nil {
		return nil, err
	}
	var sig Signature
	if err := json.Unmarshal(data, &sig); err != nil {
		return nil, err
	}
	return &sig, nil
}

type TrustLevel string

const (
	TrustLevelUnknown   TrustLevel = "unknown"
	TrustLevelUntrusted TrustLevel = "untrusted"
	TrustLevelTrusted   TrustLevel = "trusted"
	TrustLevelVerified  TrustLevel = "verified"
)

type SignerInfo struct {
	KeyID       string     `json:"key_id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Fingerprint string     `json:"fingerprint"`
	TrustLevel  TrustLevel `json:"trust_level"`
	AddedAt     time.Time  `json:"added_at"`
}

type TrustStore struct {
	signers map[string]*SignerInfo
}

func NewTrustStore() *TrustStore {
	return &TrustStore{
		signers: make(map[string]*SignerInfo),
	}
}

func (ts *TrustStore) AddSigner(keyID string, info *SignerInfo) {
	ts.signers[keyID] = info
}

func (ts *TrustStore) GetSigner(keyID string) (*SignerInfo, bool) {
	info, ok := ts.signers[keyID]
	return info, ok
}

func (ts *TrustStore) IsTrusted(keyID string) bool {
	info, ok := ts.signers[keyID]
	if !ok {
		return false
	}
	return info.TrustLevel == TrustLevelTrusted || info.TrustLevel == TrustLevelVerified
}

func (ts *TrustStore) ListSigners() []*SignerInfo {
	list := make([]*SignerInfo, 0, len(ts.signers))
	for _, info := range ts.signers {
		list = append(list, info)
	}
	return list
}

func (ts *TrustStore) RemoveSigner(keyID string) {
	delete(ts.signers, keyID)
}

func (ts *TrustStore) Save(path string) error {
	data, err := json.MarshalIndent(ts.signers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (ts *TrustStore) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &ts.signers)
}

type PluginChecksum struct {
	Algorithm string            `json:"algorithm"`
	Value     string            `json:"value"`
	Files     map[string]string `json:"files"`
}

func GenerateChecksum(dir string, algorithm string) (*PluginChecksum, error) {
	if algorithm == "" {
		algorithm = "sha256"
	}
	files := make(map[string]string)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		files[relPath] = fmt.Sprintf("%x", hash)
		return nil
	})
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	for _, v := range files {
		buf.WriteString(v)
	}
	hash := sha256.Sum256(buf.Bytes())

	return &PluginChecksum{
		Algorithm: algorithm,
		Value:     fmt.Sprintf("%x", hash),
		Files:     files,
	}, nil
}

func VerifyChecksum(dir string, checksum *PluginChecksum) (bool, error) {
	for relPath, expectedHash := range checksum.Files {
		path := filepath.Join(dir, relPath)
		data, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("file not found: %s", relPath)
		}
		hash := sha256.Sum256(data)
		actualHash := fmt.Sprintf("%x", hash)
		if actualHash != expectedHash {
			return false, fmt.Errorf("checksum mismatch for %s", relPath)
		}
	}
	return true, nil
}
