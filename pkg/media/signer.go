package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type URLSignerConfig struct {
	SecretKey     string
	DefaultExpiry time.Duration
	MaxExpiry     time.Duration
}

func DefaultURLSignerConfig() URLSignerConfig {
	return URLSignerConfig{
		SecretKey:     generateSignerSecret(),
		DefaultExpiry: 24 * time.Hour,
		MaxExpiry:     7 * 24 * time.Hour,
	}
}

type URLSigner struct {
	mu          sync.RWMutex
	secretKey   []byte
	defaultExp  time.Duration
	maxExpiry   time.Duration
	revokedKeys map[string]time.Time
}

func NewURLSigner(cfg URLSignerConfig) *URLSigner {
	secret := []byte(cfg.SecretKey)
	if len(secret) == 0 {
		secret = []byte(generateSignerSecret())
	}

	maxExp := cfg.MaxExpiry
	if maxExp <= 0 {
		maxExp = 7 * 24 * time.Hour
	}

	defExp := cfg.DefaultExpiry
	if defExp <= 0 {
		defExp = 24 * time.Hour
	}
	if defExp > maxExp {
		defExp = maxExp
	}

	return &URLSigner{
		secretKey:   secret,
		defaultExp:  defExp,
		maxExpiry:   maxExp,
		revokedKeys: make(map[string]time.Time),
	}
}

func (s *URLSigner) SetSecretKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secretKey = []byte(key)
}

func (s *URLSigner) SetDefaultExpiry(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d > s.maxExpiry {
		d = s.maxExpiry
	}
	s.defaultExp = d
}

func (s *URLSigner) SetMaxExpiry(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxExpiry = d
	if s.defaultExp > d {
		s.defaultExp = d
	}
}

func (s *URLSigner) SignURL(baseURL string, key string, expires time.Duration, metadata map[string]string) (string, error) {
	s.mu.RLock()
	secret := s.secretKey
	defExp := s.defaultExp
	maxExp := s.maxExpiry
	s.mu.RUnlock()

	if expires <= 0 {
		expires = defExp
	}
	if expires > maxExp {
		return "", fmt.Errorf("expiry %s exceeds maximum allowed %s", expires, maxExp)
	}

	now := time.Now().UTC()
	expireAt := now.Add(expires)

	params := url.Values{}
	params.Set("key", key)
	params.Set("exp", strconv.FormatInt(expireAt.Unix(), 10))
	params.Set("iat", strconv.FormatInt(now.Unix(), 10))

	if metadata != nil {
		for k, v := range metadata {
			if strings.HasPrefix(k, "m_") {
				params.Set(k, v)
			}
		}
	}

	stringToSign := key + "|" + strconv.FormatInt(expireAt.Unix(), 10) + "|" + strconv.FormatInt(now.Unix(), 10)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(stringToSign))
	signature := hex.EncodeToString(mac.Sum(nil))

	params.Set("sig", signature)

	separator := "?"
	if strings.Contains(baseURL, "?") {
		separator = "&"
	}

	return baseURL + separator + params.Encode(), nil
}

func (s *URLSigner) VerifyURL(signedURL string) (string, time.Time, map[string]string, error) {
	s.mu.RLock()
	secret := s.secretKey
	revoked := make(map[string]time.Time, len(s.revokedKeys))
	for k, v := range s.revokedKeys {
		revoked[k] = v
	}
	s.mu.RUnlock()

	u, err := url.Parse(signedURL)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("parse URL: %w", err)
	}

	query := u.Query()

	key := query.Get("key")
	expStr := query.Get("exp")
	iatStr := query.Get("iat")
	sig := query.Get("sig")

	if key == "" || expStr == "" || sig == "" {
		return "", time.Time{}, nil, fmt.Errorf("missing required signing parameters")
	}

	expireAt, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("invalid expiry: %w", err)
	}

	if time.Now().UTC().Unix() > expireAt {
		return "", time.Time{}, nil, fmt.Errorf("URL expired at %s", time.Unix(expireAt, 0).UTC().Format(time.RFC3339))
	}

	stringToSign := key + "|" + expStr + "|" + iatStr

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(stringToSign))
	expectedSig := mac.Sum(nil)

	gotSig, err := hex.DecodeString(sig)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("invalid signature format: %w", err)
	}

	if subtle.ConstantTimeCompare(expectedSig, gotSig) != 1 {
		return "", time.Time{}, nil, fmt.Errorf("invalid signature")
	}

	sigKey := key + ":" + expStr
	if revokedAt, ok := revoked[sigKey]; ok {
		return "", time.Time{}, nil, fmt.Errorf("URL revoked at %s", revokedAt.Format(time.RFC3339))
	}

	metadata := make(map[string]string)
	for k, v := range query {
		if strings.HasPrefix(k, "m_") && len(v) > 0 {
			metadata[k] = v[0]
		}
	}

	expireTime := time.Unix(expireAt, 0).UTC()

	u.RawQuery = ""

	return u.String(), expireTime, metadata, nil
}

func (s *URLSigner) RevokeURL(signedURL string) error {
	s.mu.RLock()
	secret := s.secretKey
	s.mu.RUnlock()

	u, err := url.Parse(signedURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	query := u.Query()
	key := query.Get("key")
	expStr := query.Get("exp")
	sig := query.Get("sig")

	if key == "" || expStr == "" || sig == "" {
		return fmt.Errorf("not a signed URL")
	}

	stringToSign := key + "|" + expStr + "|" + query.Get("iat")
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(stringToSign))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if sig != expectedSig {
		return fmt.Errorf("invalid signature, cannot revoke")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedKeys[key+":"+expStr] = time.Now().UTC()

	return nil
}

func (s *URLSigner) IsRevoked(signedURL string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, err := url.Parse(signedURL)
	if err != nil {
		return false, fmt.Errorf("parse URL: %w", err)
	}

	query := u.Query()
	key := query.Get("key")
	expStr := query.Get("exp")

	if key == "" || expStr == "" {
		return false, fmt.Errorf("not a signed URL")
	}

	_, ok := s.revokedKeys[key+":"+expStr]
	return ok, nil
}

func (s *URLSigner) CleanupRevoked() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	count := 0
	for sigKey, revokedAt := range s.revokedKeys {
		if now.Sub(revokedAt) > 24*time.Hour {
			delete(s.revokedKeys, sigKey)
			count++
		}
	}
	return count
}

func (s *URLSigner) RemainingTime(signedURL string) (time.Duration, error) {
	u, err := url.Parse(signedURL)
	if err != nil {
		return 0, fmt.Errorf("parse URL: %w", err)
	}

	query := u.Query()
	expStr := query.Get("exp")
	if expStr == "" {
		return 0, fmt.Errorf("no expiry in URL")
	}

	expireAt, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid expiry: %w", err)
	}

	remaining := time.Unix(expireAt, 0).UTC().Sub(time.Now().UTC())
	if remaining < 0 {
		return 0, fmt.Errorf("URL expired")
	}

	return remaining, nil
}

func (s *URLSigner) SignStorageObject(storage StorageBackend, key string, expires time.Duration) (string, error) {
	baseURL, err := storage.URL(nil, key, 0)
	if err != nil {
		return "", fmt.Errorf("get base URL: %w", err)
	}

	return s.SignURL(baseURL, key, expires, nil)
}

func generateSignerSecret() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i * 7 % 256)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

type SignedURLResult struct {
	URL       string
	ExpiresAt time.Time
	Remaining time.Duration
	Key       string
	Metadata  map[string]string
	IsRevoked bool
}

type URLSignerManager struct {
	signers map[string]*URLSigner
	mu      sync.RWMutex
	defName string
}

func NewURLSignerManager() *URLSignerManager {
	return &URLSignerManager{
		signers: make(map[string]*URLSigner),
	}
}

func (m *URLSignerManager) Register(name string, signer *URLSigner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signers[name] = signer
	if m.defName == "" {
		m.defName = name
	}
}

func (m *URLSignerManager) SetDefault(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defName = name
}

func (m *URLSignerManager) Get(name string) *URLSigner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signers[name]
}

func (m *URLSignerManager) Default() *URLSigner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signers[m.defName]
}

func (m *URLSignerManager) Sign(key string, expires time.Duration) (string, error) {
	signer := m.Default()
	if signer == nil {
		return "", fmt.Errorf("no URL signer configured")
	}
	return signer.SignURL("https://media.example.com/"+key, key, expires, nil)
}

func (m *URLSignerManager) Verify(signedURL string) (*SignedURLResult, error) {
	signer := m.Default()
	if signer == nil {
		return nil, fmt.Errorf("no URL signer configured")
	}

	baseURL, expireTime, metadata, err := signer.VerifyURL(signedURL)
	if err != nil {
		return nil, err
	}

	remaining := expireTime.Sub(time.Now().UTC())
	isRevoked := remaining < 0

	return &SignedURLResult{
		URL:       baseURL,
		ExpiresAt: expireTime,
		Remaining: remaining,
		Key:       "",
		Metadata:  metadata,
		IsRevoked: isRevoked,
	}, nil
}

func (m *URLSignerManager) Revoke(signedURL string) error {
	signer := m.Default()
	if signer == nil {
		return fmt.Errorf("no URL signer configured")
	}
	return signer.RevokeURL(signedURL)
}
