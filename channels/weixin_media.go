package channels

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
)

// WeixinMediaHandler handles media file operations
type WeixinMediaHandler struct {
	apiClient  *WeixinAPIClient
	cdnBaseURL string
	httpClient *http.Client
}

// NewWeixinMediaHandler creates a new media handler
func NewWeixinMediaHandler(apiClient *WeixinAPIClient, cdnBaseURL string) *WeixinMediaHandler {
	if cdnBaseURL == "" {
		cdnBaseURL = DefaultWeixinCDNURL
	}
	return &WeixinMediaHandler{
		apiClient:  apiClient,
		cdnBaseURL: cdnBaseURL,
		httpClient: &http.Client{},
	}
}

// PKCS7Pad applies PKCS7 padding to data
func PKCS7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// PKCS7Unpad removes PKCS7 padding from data
func PKCS7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[length-1])
	if padding > length {
		return nil, fmt.Errorf("invalid padding")
	}
	return data[:length-padding], nil
}

// AESEncrypt encrypts data using AES-128-ECB with PKCS7 padding
func AESEncrypt(plaintext, key []byte) ([]byte, error) {
	if len(key) > 16 {
		key = key[:16]
	} else if len(key) < 16 {
		paddedKey := make([]byte, 16)
		copy(paddedKey, key)
		key = paddedKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Apply PKCS7 padding
	paddedData := PKCS7Pad(plaintext, aes.BlockSize)

	// Encrypt in ECB mode
	ciphertext := make([]byte, len(paddedData))
	for i := 0; i < len(paddedData); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], paddedData[i:i+aes.BlockSize])
	}

	return ciphertext, nil
}

// AESDecrypt decrypts data using AES-128-ECB and removes PKCS7 padding
func AESDecrypt(ciphertext, key []byte) ([]byte, error) {
	if len(key) > 16 {
		key = key[:16]
	} else if len(key) < 16 {
		paddedKey := make([]byte, 16)
		copy(paddedKey, key)
		key = paddedKey
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of block size")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Decrypt in ECB mode
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}

	// Remove PKCS7 padding
	return PKCS7Unpad(plaintext)
}

// DownloadMedia downloads and decrypts media from CDN using encrypt_query_param
func (h *WeixinMediaHandler) DownloadMedia(ctx context.Context, encryptQueryParam, aesKeyBase64 string) ([]byte, error) {
	// Download encrypted data from CDN
	cdnURL := h.cdnBaseURL + "/" + encryptQueryParam

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download from CDN: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("CDN download failed with status %d", resp.StatusCode)
	}

	encryptedData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read CDN response: %w", err)
	}

	// Decode AES key
	aesKey, err := base64.StdEncoding.DecodeString(aesKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode AES key: %w", err)
	}

	// Decrypt data
	decryptedData, err := AESDecrypt(encryptedData, aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return decryptedData, nil
}

// UploadMedia uploads media to CDN and returns upload_param
func (h *WeixinMediaHandler) UploadMedia(ctx context.Context, data []byte, fileKey, toUserID string, mediaType int) (string, string, error) {
	// TODO: Implement proper upload flow:
	// 1. Generate AES key
	// 2. Encrypt data with AES-128-ECB
	// 3. Calculate MD5 and sizes
	// 4. Call getUploadUrl
	// 5. PUT encrypted data to CDN URL
	// 6. Return upload_param

	return "", "", fmt.Errorf("not implemented")
}

// UploadFile uploads a file to Weixin CDN (placeholder - needs proper implementation)
func (h *WeixinMediaHandler) UploadFile(ctx context.Context, filePath, toUserID string) error {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	_ = data // TODO: Implement upload
	return fmt.Errorf("file upload not implemented yet")
}

// UploadData uploads data to Weixin CDN (placeholder - needs proper implementation)
func (h *WeixinMediaHandler) UploadData(ctx context.Context, data []byte, fileName, toUserID string) error {
	_ = data
	_ = fileName
	_ = toUserID
	return fmt.Errorf("data upload not implemented yet")
}

// DownloadFile downloads and decrypts a file from CDN
func (h *WeixinMediaHandler) DownloadFile(ctx context.Context, encryptQueryParam, aesKeyBase64 string) ([]byte, error) {
	return h.DownloadMedia(ctx, encryptQueryParam, aesKeyBase64)
}

// DownloadToBase64 downloads a file and returns as base64 string
func (h *WeixinMediaHandler) DownloadToBase64(ctx context.Context, encryptQueryParam, aesKeyBase64 string) (string, error) {
	data, err := h.DownloadMedia(ctx, encryptQueryParam, aesKeyBase64)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
