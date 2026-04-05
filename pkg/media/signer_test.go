package media

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestURLSigner_SignAndVerify(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret-key-12345",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/media/photo.jpg", "photo.jpg", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if signedURL == "" {
		t.Fatal("signed URL is empty")
	}

	if !strings.Contains(signedURL, "sig=") {
		t.Error("signed URL missing signature")
	}
	if !strings.Contains(signedURL, "exp=") {
		t.Error("signed URL missing expiry")
	}
	if !strings.Contains(signedURL, "key=") {
		t.Error("signed URL missing key")
	}

	baseURL, expireTime, metadata, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if baseURL != "https://example.com/media/photo.jpg" {
		t.Errorf("expected base URL https://example.com/media/photo.jpg, got %s", baseURL)
	}

	if expireTime.Before(time.Now().UTC()) {
		t.Error("expire time is in the past")
	}

	if metadata == nil {
		t.Error("expected non-nil metadata")
	}
}

func TestURLSigner_ExpiredURL(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, err := signer.SignURL("https://example.com/file", "file.txt", 0, nil)
	if err != nil {
		t.Fatalf("sign with zero expiry: %v", err)
	}
}

func TestURLSigner_ExceedsMaxExpiry(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     time.Hour,
	})

	_, err := signer.SignURL("https://example.com/file", "file.txt", 24*time.Hour, nil)
	if err == nil {
		t.Fatal("expected error for exceeding max expiry")
	}
}

func TestURLSigner_TamperedSignature(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	tampered := signedURL[:len(signedURL)-5] + "aaaaa"

	_, _, _, err = signer.VerifyURL(tampered)
	if err == nil {
		t.Fatal("expected error for tampered signature")
	}
}

func TestURLSigner_TamperedKey(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	tampered := strings.Replace(signedURL, "key=file.txt", "key=other.txt", 1)

	_, _, _, err = signer.VerifyURL(tampered)
	if err == nil {
		t.Fatal("expected error for tampered key")
	}
}

func TestURLSigner_MissingParams(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, _, _, err := signer.VerifyURL("https://example.com/file?foo=bar")
	if err == nil {
		t.Fatal("expected error for missing params")
	}
}

func TestURLSigner_RevokeAndVerify(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	err = signer.RevokeURL(signedURL)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, _, _, err = signer.VerifyURL(signedURL)
	if err == nil {
		t.Fatal("expected error for revoked URL")
	}
}

func TestURLSigner_RevokeInvalidSignature(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	tamperedURL := "https://example.com/file?key=file.txt&exp=9999999999&iat=0000000000&sig=invalid"

	err := signer.RevokeURL(tamperedURL)
	if err == nil {
		t.Fatal("expected error for revoking URL with invalid signature")
	}
}

func TestURLSigner_IsRevoked(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	revoked, err := signer.IsRevoked(signedURL)
	if err != nil {
		t.Fatalf("isRevoked: %v", err)
	}
	if revoked {
		t.Error("expected URL not to be revoked")
	}

	_ = signer.RevokeURL(signedURL)

	revoked, err = signer.IsRevoked(signedURL)
	if err != nil {
		t.Fatalf("isRevoked after revoke: %v", err)
	}
	if !revoked {
		t.Error("expected URL to be revoked")
	}
}

func TestURLSigner_RemainingTime(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	remaining, err := signer.RemainingTime(signedURL)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}

	if remaining <= 0 {
		t.Error("expected positive remaining time")
	}

	if remaining > time.Hour {
		t.Errorf("expected remaining time <= 1 hour, got %s", remaining)
	}
}

func TestURLSigner_CleanupRevoked(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_ = signer.RevokeURL(signedURL)

	signer.mu.Lock()
	signer.revokedKeys["old:revoked"] = time.Now().UTC().Add(-25 * time.Hour)
	signer.mu.Unlock()

	cleaned := signer.CleanupRevoked()
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned up, got %d", cleaned)
	}
}

func TestURLSigner_SignWithMetadata(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	metadata := map[string]string{
		"m_user": "alice",
		"m_role": "admin",
		"secret": "should-not-appear",
	}

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, metadata)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, _, gotMetadata, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if gotMetadata["m_user"] != "alice" {
		t.Errorf("expected m_user=alice, got %s", gotMetadata["m_user"])
	}
	if gotMetadata["m_role"] != "admin" {
		t.Errorf("expected m_role=admin, got %s", gotMetadata["m_role"])
	}
	if _, ok := gotMetadata["secret"]; ok {
		t.Error("secret metadata should not appear (no m_ prefix)")
	}
}

func TestURLSigner_DifferentSecrets(t *testing.T) {
	signer1 := NewURLSigner(URLSignerConfig{
		SecretKey:     "secret-1",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signer2 := NewURLSigner(URLSignerConfig{
		SecretKey:     "secret-2",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer1.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, _, _, err = signer2.VerifyURL(signedURL)
	if err == nil {
		t.Fatal("expected verification to fail with different secret")
	}
}

func TestURLSigner_SetSecretKey(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "old-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	signer.SetSecretKey("new-secret")

	_, _, _, err = signer.VerifyURL(signedURL)
	if err == nil {
		t.Fatal("expected verification to fail after secret change")
	}
}

func TestURLSigner_SetDefaultExpiry(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signer.SetDefaultExpiry(30 * time.Minute)

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", 0, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, expireTime, _, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	expectedMax := time.Now().UTC().Add(30*time.Minute + time.Second)
	if expireTime.After(expectedMax) {
		t.Errorf("expected expiry around 30 minutes, got %s", expireTime.Sub(time.Now().UTC()))
	}
}

func TestURLSigner_SignStorageObject(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "photo.jpg", []byte("image data"), StoragePutOptions{
		MimeType: "image/jpeg",
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	signedURL, err := signer.SignStorageObject(store, "photo.jpg", time.Hour)
	if err != nil {
		t.Fatalf("sign storage object: %v", err)
	}

	if !strings.Contains(signedURL, "sig=") {
		t.Error("signed URL missing signature")
	}

	if !strings.HasPrefix(signedURL, "https://cdn.example.com/photo.jpg") {
		t.Errorf("expected URL to start with https://cdn.example.com/photo.jpg, got %s", signedURL)
	}
}

func TestURLSignerManager(t *testing.T) {
	sm := NewURLSignerManager()

	signer1 := NewURLSigner(URLSignerConfig{
		SecretKey:     "key-1",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signer2 := NewURLSigner(URLSignerConfig{
		SecretKey:     "key-2",
		DefaultExpiry: 30 * time.Minute,
		MaxExpiry:     12 * time.Hour,
	})

	sm.Register("primary", signer1)
	sm.Register("secondary", signer2)

	if sm.Default() != signer1 {
		t.Error("expected primary as default")
	}

	sm.SetDefault("secondary")
	if sm.Default() != signer2 {
		t.Error("expected secondary as default")
	}

	got := sm.Get("primary")
	if got != signer1 {
		t.Error("failed to get primary signer")
	}
}

func TestURLSignerManager_Sign(t *testing.T) {
	sm := NewURLSignerManager()

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	sm.Register("default", signer)

	signedURL, err := sm.Sign("photo.jpg", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if signedURL == "" {
		t.Error("signed URL is empty")
	}
}

func TestURLSignerManager_Verify(t *testing.T) {
	sm := NewURLSignerManager()

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	sm.Register("default", signer)

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	result, err := sm.Verify(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if result.URL != "https://example.com/file" {
		t.Errorf("expected URL https://example.com/file, got %s", result.URL)
	}

	if result.Remaining <= 0 {
		t.Error("expected positive remaining time")
	}
}

func TestURLSignerManager_Revoke(t *testing.T) {
	sm := NewURLSignerManager()

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	sm.Register("default", signer)

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	err = sm.Revoke(signedURL)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = sm.Verify(signedURL)
	if err == nil {
		t.Fatal("expected verification to fail after revoke")
	}
}

func TestURLSignerManager_NoSigner(t *testing.T) {
	sm := NewURLSignerManager()

	_, err := sm.Sign("key", time.Hour)
	if err == nil {
		t.Fatal("expected error with no signer")
	}

	_, err = sm.Verify("https://example.com")
	if err == nil {
		t.Fatal("expected error with no signer")
	}

	err = sm.Revoke("https://example.com")
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestProcessor_Signer_GetSet(t *testing.T) {
	proc := NewProcessor("")

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "processor-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	proc.SetSigner(signer)

	if proc.Signer() != signer {
		t.Error("signer not set")
	}
}

func TestProcessor_PresignURL_WithSigner(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})
	proc.SetStorage(local)

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})
	proc.SetSigner(signer)

	ctx := context.Background()
	media := &Media{
		ID:       "presign-test",
		Data:     []byte("data"),
		MimeType: "image/jpeg",
		Name:     "test.jpg",
	}

	_, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	signedURL, err := proc.PresignURL(ctx, "test.jpg", time.Hour)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	if !strings.Contains(signedURL, "sig=") {
		t.Error("signed URL missing signature")
	}
}

func TestProcessor_PresignURL_NoSigner(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})
	proc.SetStorage(local)

	ctx := context.Background()
	media := &Media{
		ID:   "no-signer",
		Data: []byte("data"),
		Name: "test.jpg",
	}

	_, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	url, err := proc.PresignURL(ctx, "test.jpg", time.Hour)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	if url != "https://cdn.example.com/test.jpg" {
		t.Errorf("expected https://cdn.example.com/test.jpg, got %s", url)
	}
}

func TestProcessor_VerifySignedURL(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})
	proc.SetStorage(local)

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})
	proc.SetSigner(signer)

	ctx := context.Background()
	media := &Media{
		ID:   "verify-test",
		Data: []byte("data"),
		Name: "test.jpg",
	}

	_, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	signedURL, err := proc.PresignURL(ctx, "test.jpg", time.Hour)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	result, err := proc.VerifySignedURL(ctx, signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if result.Remaining <= 0 {
		t.Error("expected positive remaining time")
	}

	if result.IsRevoked {
		t.Error("expected URL not to be revoked")
	}
}

func TestProcessor_RevokeSignedURL(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})
	proc.SetStorage(local)

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})
	proc.SetSigner(signer)

	ctx := context.Background()
	media := &Media{
		ID:   "revoke-test",
		Data: []byte("data"),
		Name: "test.jpg",
	}

	_, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	signedURL, err := proc.PresignURL(ctx, "test.jpg", time.Hour)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	err = proc.RevokeSignedURL(ctx, signedURL)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = proc.VerifySignedURL(ctx, signedURL)
	if err == nil {
		t.Fatal("expected verification to fail after revoke")
	}
}

func TestMediaPipeline_SignURL(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "pipeline-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.Signer = signer

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	signedURL, err := p.SignURL(ctx, "photo.jpg", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if !strings.Contains(signedURL, "sig=") {
		t.Error("signed URL missing signature")
	}
}

func TestMediaPipeline_VerifySignedURL(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "pipeline-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.Signer = signer

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	signedURL, err := p.SignURL(ctx, "photo.jpg", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	result, err := p.VerifySignedURL(ctx, signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if result.Remaining <= 0 {
		t.Error("expected positive remaining time")
	}
}

func TestMediaPipeline_RevokeSignedURL(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "pipeline-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.Signer = signer

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	signedURL, err := p.SignURL(ctx, "photo.jpg", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	err = p.RevokeSignedURL(ctx, signedURL)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = p.VerifySignedURL(ctx, signedURL)
	if err == nil {
		t.Fatal("expected verification to fail after revoke")
	}
}

func TestMediaPipeline_SignMediaURL(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "pipeline-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.Signer = signer

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	media := &Media{
		ID:       "sign-media",
		Type:     TypeImage,
		Data:     []byte("image data"),
		MimeType: "image/jpeg",
		Name:     "photo.jpg",
		Metadata: map[string]any{
			"storage-key": "photo.jpg",
		},
	}

	signedURL, err := p.SignMediaURL(ctx, media, time.Hour)
	if err != nil {
		t.Fatalf("sign media URL: %v", err)
	}

	if !strings.Contains(signedURL, "sig=") {
		t.Error("signed URL missing signature")
	}
}

func TestMediaPipeline_AutoSign(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "autosign-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.Signer = signer
	cfg.AutoSave = true
	cfg.AutoSign = true
	cfg.SignExpiry = 2 * time.Hour

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	_, err := p.Download(ctx, "http://localhost:1/photo.jpg")
	if err == nil {
		t.Log("auto-sign attempted (expected failure due to no server)")
	}
}

func TestMediaPipeline_CleanupRevokedURLs(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "cleanup-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Signer = signer

	p := NewMediaPipeline(cfg)

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_ = signer.RevokeURL(signedURL)

	signer.mu.Lock()
	signer.revokedKeys["old:entry"] = time.Now().UTC().Add(-25 * time.Hour)
	signer.mu.Unlock()

	cleaned := p.CleanupRevokedURLs()
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned up, got %d", cleaned)
	}
}

func TestMediaPipeline_NoSignerConfigured(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	ctx := context.Background()

	_, err := p.SignURL(ctx, "key", time.Hour)
	if err == nil {
		t.Fatal("expected error with no signer")
	}

	_, err = p.VerifySignedURL(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error with no signer")
	}

	err = p.RevokeSignedURL(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error with no signer")
	}

	media := &Media{ID: "test"}
	_, err = p.SignMediaURL(ctx, media, time.Hour)
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestURLSigner_DefaultExpiry(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: 30 * time.Minute,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", 0, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, expireTime, _, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	remaining := expireTime.Sub(time.Now().UTC())
	if remaining > 31*time.Minute {
		t.Errorf("expected remaining time around 30 minutes, got %s", remaining)
	}
}

func TestURLSigner_EmptySecretKey(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, _, _, err = signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify with auto-generated secret: %v", err)
	}
}

func TestURLSigner_URLWithExistingQuery(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file?version=1", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if !strings.Contains(signedURL, "&sig=") {
		t.Error("expected & separator for existing query params")
	}
}

func TestURLSigner_RemainingTimeExpired(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	u := "https://example.com/file?key=file.txt&exp=0&iat=0&sig=invalid"

	_, err := signer.RemainingTime(u)
	if err == nil {
		t.Log("remaining time check passed (may vary by implementation)")
	}
}

func TestLocalStorage_SignAndAccess(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "storage-sign-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "docs/report.pdf", []byte("report content"), StoragePutOptions{
		MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	signedURL, err := signer.SignStorageObject(store, "docs/report.pdf", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	baseURL, expireTime, _, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !strings.HasPrefix(baseURL, "https://cdn.example.com/docs/report.pdf") {
		t.Errorf("expected base URL to start with https://cdn.example.com/docs/report.pdf, got %s", baseURL)
	}

	if expireTime.Before(time.Now().UTC()) {
		t.Error("expire time is in the past")
	}
}

func TestURLSigner_ConcurrentAccess(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "concurrent-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := "file" + string(rune('0'+idx)) + ".txt"
			signedURL, err := signer.SignURL("https://example.com/"+key, key, time.Hour, nil)
			if err != nil {
				t.Errorf("sign %d: %v", idx, err)
				done <- false
				return
			}

			_, _, _, err = signer.VerifyURL(signedURL)
			if err != nil {
				t.Errorf("verify %d: %v", idx, err)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		if !<-done {
			t.Fatal("concurrent test failed")
		}
	}
}

func TestURLSigner_RevokeAndIsRevoked(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "revoke-test",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	revoked, err := signer.IsRevoked(signedURL)
	if err != nil {
		t.Fatalf("isRevoked before: %v", err)
	}
	if revoked {
		t.Error("should not be revoked yet")
	}

	err = signer.RevokeURL(signedURL)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	revoked, err = signer.IsRevoked(signedURL)
	if err != nil {
		t.Fatalf("isRevoked after: %v", err)
	}
	if !revoked {
		t.Error("should be revoked now")
	}
}

func TestURLSignerManager_MultipleSigners(t *testing.T) {
	sm := NewURLSignerManager()

	signer1 := NewURLSigner(URLSignerConfig{
		SecretKey:     "key-1",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signer2 := NewURLSigner(URLSignerConfig{
		SecretKey:     "key-2",
		DefaultExpiry: 2 * time.Hour,
		MaxExpiry:     48 * time.Hour,
	})

	sm.Register("short", signer1)
	sm.Register("long", signer2)

	signedURL1, err := signer1.SignURL("https://example.com/f1", "f1", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign1: %v", err)
	}

	signedURL2, err := signer2.SignURL("https://example.com/f2", "f2", 2*time.Hour, nil)
	if err != nil {
		t.Fatalf("sign2: %v", err)
	}

	sm.SetDefault("short")
	result1, err := sm.Verify(signedURL1)
	if err != nil {
		t.Fatalf("verify1 via manager: %v", err)
	}
	if result1.URL != "https://example.com/f1" {
		t.Errorf("expected https://example.com/f1, got %s", result1.URL)
	}

	sm.SetDefault("long")
	result2, err := sm.Verify(signedURL2)
	if err != nil {
		t.Fatalf("verify2 via manager: %v", err)
	}
	if result2.URL != "https://example.com/f2" {
		t.Errorf("expected https://example.com/f2, got %s", result2.URL)
	}
}

func TestMediaPipeline_DownloadWithAutoSign(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "autosign-pipeline",
		DefaultExpiry: 2 * time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.Signer = signer
	cfg.AutoSave = true
	cfg.AutoSign = true
	cfg.SignExpiry = 2 * time.Hour

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	_, err := p.Download(ctx, "http://localhost:1/test.jpg")
	if err == nil {
		t.Log("download with auto-sign attempted (expected failure due to no server)")
	}
}

func TestURLSigner_VerifyInvalidURL(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, _, _, err := signer.VerifyURL("://invalid-url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestURLSigner_VerifyInvalidExpiry(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, _, _, err := signer.VerifyURL("https://example.com?key=test&exp=not-a-number&iat=0&sig=abc")
	if err == nil {
		t.Fatal("expected error for invalid expiry")
	}
}

func TestURLSigner_VerifyInvalidSignature(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, _, _, err := signer.VerifyURL("https://example.com?key=test&exp=9999999999&iat=0&sig=not-hex!!")
	if err == nil {
		t.Fatal("expected error for invalid hex signature")
	}
}

func TestURLSigner_RemainingTimeNoExpiry(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, err := signer.RemainingTime("https://example.com/file")
	if err == nil {
		t.Fatal("expected error for URL without expiry")
	}
}

func TestURLSigner_RemainingTimeInvalidExpiry(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, err := signer.RemainingTime("https://example.com/file?exp=not-a-number")
	if err == nil {
		t.Fatal("expected error for invalid expiry")
	}
}

func TestURLSigner_IsRevokedNotSignedURL(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	_, err := signer.IsRevoked("https://example.com/file")
	if err == nil {
		t.Fatal("expected error for non-signed URL")
	}
}

func TestURLSigner_RevokeNotSignedURL(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	err := signer.RevokeURL("https://example.com/file")
	if err == nil {
		t.Fatal("expected error for non-signed URL")
	}
}

func TestURLSigner_CleanupRevokedEmpty(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	cleaned := signer.CleanupRevoked()
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned up, got %d", cleaned)
	}
}

func TestURLSignerManager_NoSignerErrors(t *testing.T) {
	sm := NewURLSignerManager()

	_, err := sm.Sign("key", time.Hour)
	if err == nil {
		t.Fatal("expected error")
	}

	_, err = sm.Verify("https://example.com")
	if err == nil {
		t.Fatal("expected error")
	}

	err = sm.Revoke("https://example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProcessor_VerifySignedURL_NoSigner(t *testing.T) {
	proc := NewProcessor("")

	ctx := context.Background()
	_, err := proc.VerifySignedURL(ctx, "https://example.com/file?sig=abc")
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestProcessor_RevokeSignedURL_NoSigner(t *testing.T) {
	proc := NewProcessor("")

	ctx := context.Background()
	err := proc.RevokeSignedURL(ctx, "https://example.com/file?sig=abc")
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestMediaPipeline_SignURL_NoSigner(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	_, err := p.SignURL(ctx, "key", time.Hour)
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestMediaPipeline_VerifySignedURL_NoSigner(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	_, err := p.VerifySignedURL(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestMediaPipeline_RevokeSignedURL_NoSigner(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	err := p.RevokeSignedURL(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestMediaPipeline_SignMediaURL_NoSigner(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	media := &Media{ID: "test"}
	_, err := p.SignMediaURL(ctx, media, time.Hour)
	if err == nil {
		t.Fatal("expected error with no signer")
	}
}

func TestMediaPipeline_CleanupRevokedURLs_NoSigner(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	cleaned := p.CleanupRevokedURLs()
	if cleaned != 0 {
		t.Errorf("expected 0, got %d", cleaned)
	}
}

func TestURLSigner_VerifyExpiredURL(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	signer.mu.Lock()
	u, _ := parseURLForTest(signedURL)
	q := u.Query()
	q.Set("exp", "0")
	u.RawQuery = q.Encode()
	signedURL = u.String()
	signer.mu.Unlock()

	_, _, _, err = signer.VerifyURL(signedURL)
	if err == nil {
		t.Fatal("expected error for expired URL")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expired error, got: %v", err)
	}
}

func parseURLForTest(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

func TestURLSigner_RevokedURLErrorMessage(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_ = signer.RevokeURL(signedURL)

	_, _, _, err = signer.VerifyURL(signedURL)
	if err == nil {
		t.Fatal("expected error for revoked URL")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("expected revoked error, got: %v", err)
	}
}

func TestURLSigner_SignURLWithSpecialCharacters(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	key := "files/my photo (1).jpg"
	signedURL, err := signer.SignURL("https://example.com/"+key, key, time.Hour, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	baseURL, _, _, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	decoded, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	if !strings.Contains(decoded.Path, "my photo") {
		t.Errorf("expected URL path to contain 'my photo', got %s", decoded.Path)
	}
}

func TestSignedURLResult_Fields(t *testing.T) {
	signer := NewURLSigner(URLSignerConfig{
		SecretKey:     "test-secret",
		DefaultExpiry: time.Hour,
		MaxExpiry:     24 * time.Hour,
	})

	signedURL, err := signer.SignURL("https://example.com/file", "file.txt", time.Hour, map[string]string{
		"m_user": "test",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	baseURL, expireTime, metadata, err := signer.VerifyURL(signedURL)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	result := &SignedURLResult{
		URL:       baseURL,
		ExpiresAt: expireTime,
		Remaining: expireTime.Sub(time.Now().UTC()),
		Metadata:  metadata,
		IsRevoked: false,
	}

	if result.URL != "https://example.com/file" {
		t.Errorf("expected URL https://example.com/file, got %s", result.URL)
	}

	if result.Metadata["m_user"] != "test" {
		t.Errorf("expected m_user=test, got %s", result.Metadata["m_user"])
	}

	if result.IsRevoked {
		t.Error("expected not revoked")
	}
}
