package pairing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewPairingStore(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "feishu",
		AccountID:  "",
		DataDir:    tmpDir,
		CodeLength: 8,
		CodeExpiry: time.Hour,
		MaxPending: 3,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	if store == nil {
		t.Fatal("store is nil")
	}

	// Files are created lazily on first write
	// Create a request to ensure requests file exists
	_, _, err = store.UpsertRequest("test_user", "Test User")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}

	// Approve the request to create allowlist file
	_, _, err = store.Approve("code12345")
	if err != nil {
		// This is expected to fail since code12345 doesn't exist
		// Add directly to allowlist instead
		_ = store.AddToAllowlist("test_user2", "Test User 2")
	}

	// Check files exist
	if _, err := os.Stat(store.requestsFile); err != nil {
		t.Errorf("requests file not created: %v", err)
	}
	// Allowlist file is only created when there's an entry
	_, err = os.Stat(store.allowFile)
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("allowlist file error: %v", err)
	}
}

func TestNewPairingStoreWithAccount(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "feishu",
		AccountID:  "test-account",
		DataDir:    tmpDir,
		CodeLength: 8,
		CodeExpiry: time.Hour,
		MaxPending: 3,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Check filenames contain account ID
	expectedRequestsFile := filepath.Join(tmpDir, "feishu-test-account-pairing.json")
	if store.requestsFile != expectedRequestsFile {
		t.Errorf("requests file = %s, want %s", store.requestsFile, expectedRequestsFile)
	}

	expectedAllowFile := filepath.Join(tmpDir, "feishu-test-account-allowFrom.json")
	if store.allowFile != expectedAllowFile {
		t.Errorf("allow file = %s, want %s", store.allowFile, expectedAllowFile)
	}
}

func TestGenerateCode(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeLength: 8,
		CodeExpiry: time.Hour,
		MaxPending: 3,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Generate multiple codes and check uniqueness
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := store.generateCode()
		if err != nil {
			t.Fatalf("generateCode failed: %v", err)
		}
		if len(code) != 8 {
			t.Errorf("code length = %d, want 8", len(code))
		}
		// Check no ambiguous chars
		for _, c := range code {
			if c == '0' || c == 'O' || c == '1' || c == 'I' {
				t.Errorf("code contains ambiguous char: %s", code)
			}
		}
		codes[code] = true
	}

	// Check all unique
	if len(codes) != 100 {
		t.Errorf("generated %d unique codes, want 100", len(codes))
	}
}

func TestUpsertRequest(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 3,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Create new request
	code1, created1, err := store.UpsertRequest("user1", "Alice")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}
	if !created1 {
		t.Error("first request should be created")
	}
	if len(code1) != 8 {
		t.Errorf("code length = %d, want 8", len(code1))
	}

	// Duplicate request should return same code
	code2, created2, err := store.UpsertRequest("user1", "Alice")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}
	if created2 {
		t.Error("duplicate request should not be created")
	}
	if code2 != code1 {
		t.Errorf("duplicate code = %s, want %s", code2, code1)
	}

	// Different user should get different code
	code3, created3, err := store.UpsertRequest("user2", "Bob")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}
	if !created3 {
		t.Error("different user request should be created")
	}
	if code3 == code1 {
		t.Error("different user should get different code")
	}
}

func TestUpsertRequestMaxPending(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 2,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Create max pending requests
	_, _, err = store.UpsertRequest("user1", "Alice")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}

	_, _, err = store.UpsertRequest("user2", "Bob")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}

	// Third request should fail
	_, _, err = store.UpsertRequest("user3", "Charlie")
	if err == nil {
		t.Error("expected max pending error, got nil")
	}
}

func TestUpsertRequestExpired(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Second, // 1 second expiry (minimum due to Unix timestamp precision)
		MaxPending: 3,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Create request
	code1, created1, err := store.UpsertRequest("user1", "Alice")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}
	if !created1 {
		t.Error("first request should be created")
	}
	t.Logf("First request created with code: %s", code1)

	// Wait for expiry (more than 1 second)
	time.Sleep(time.Second * 2)

	// New request for same user should create new code (after cleaning expired)
	code2, created2, err := store.UpsertRequest("user1", "Alice")
	t.Logf("Second request: code=%s created=%v err=%v", code2, created2, err)
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}
	if !created2 {
		t.Error("request after expiry should be created")
	}
	if code2 == code1 {
		t.Error("new code should differ from expired code")
	}
}

func TestListPending(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Create some requests
	store.UpsertRequest("user1", "Alice")
	store.UpsertRequest("user2", "Bob")
	store.UpsertRequest("user3", "Charlie")

	pending := store.ListPending()
	if len(pending) != 3 {
		t.Errorf("pending count = %d, want 3", len(pending))
	}
}

func TestApprove(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Create request
	code, _, err := store.UpsertRequest("user1", "Alice")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}

	// Approve
	senderID, name, err := store.Approve(code)
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if senderID != "user1" {
		t.Errorf("senderID = %s, want user1", senderID)
	}
	if name != "Alice" {
		t.Errorf("name = %s, want Alice", name)
	}

	// Check in allowlist
	if !store.IsAllowed("user1") {
		t.Error("user1 should be in allowlist")
	}

	// Check removed from pending
	pending := store.ListPending()
	if len(pending) != 0 {
		t.Errorf("pending count = %d, want 0", len(pending))
	}
}

func TestApproveInvalidCode(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Approve non-existent code
	_, _, err = store.Approve("INVALID")
	if err == nil {
		t.Error("expected error for invalid code, got nil")
	}
}

func TestReject(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Create request
	code, _, err := store.UpsertRequest("user1", "Alice")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}

	// Reject
	err = store.Reject(code)
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Check NOT in allowlist
	if store.IsAllowed("user1") {
		t.Error("user1 should not be in allowlist after reject")
	}

	// Check removed from pending
	pending := store.ListPending()
	if len(pending) != 0 {
		t.Errorf("pending count = %d, want 0", len(pending))
	}
}

func TestIsAllowed(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Not allowed initially
	if store.IsAllowed("user1") {
		t.Error("user1 should not be allowed initially")
	}

	// Add to allowlist directly
	err = store.AddToAllowlist("user1", "Alice")
	if err != nil {
		t.Fatalf("AddToAllowlist failed: %v", err)
	}

	// Now allowed
	if !store.IsAllowed("user1") {
		t.Error("user1 should be allowed after AddToAllowlist")
	}
}

func TestGetAllowlist(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Add some users
	store.AddToAllowlist("user1", "Alice")
	store.AddToAllowlist("user2", "Bob")

	allowlist := store.GetAllowlist()
	if len(allowlist) != 2 {
		t.Errorf("allowlist size = %d, want 2", len(allowlist))
	}
	if allowlist["user1"] != "Alice" {
		t.Errorf("user1 name = %s, want Alice", allowlist["user1"])
	}
	if allowlist["user2"] != "Bob" {
		t.Errorf("user2 name = %s, want Bob", allowlist["user2"])
	}
}

func TestRemoveFromAllowlist(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store, err := NewPairingStore(cfg)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Add to allowlist
	store.AddToAllowlist("user1", "Alice")

	// Remove
	err = store.RemoveFromAllowlist("user1")
	if err != nil {
		t.Fatalf("RemoveFromAllowlist failed: %v", err)
	}

	// Check removed
	if store.IsAllowed("user1") {
		t.Error("user1 should not be allowed after removal")
	}
}

func TestLoadSave(t *testing.T) {
	tmpDir := t.TempDir()

	cfg1 := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	// Create store and add data
	store1, err := NewPairingStore(cfg1)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	store1.AddToAllowlist("user1", "Alice")
	store1.AddToAllowlist("user2", "Bob")
	_, _, err = store1.UpsertRequest("user3", "Charlie")
	if err != nil {
		t.Fatalf("UpsertRequest failed: %v", err)
	}

	// Create new store with same config (should load existing data)
	cfg2 := Config{
		Channel:    "test",
		DataDir:    tmpDir,
		CodeExpiry: time.Hour,
		MaxPending: 10,
	}

	store2, err := NewPairingStore(cfg2)
	if err != nil {
		t.Fatalf("NewPairingStore failed: %v", err)
	}

	// Check allowlist loaded
	if !store2.IsAllowed("user1") {
		t.Error("user1 should be loaded in allowlist")
	}
	if !store2.IsAllowed("user2") {
		t.Error("user2 should be loaded in allowlist")
	}

	// Check pending loaded
	pending := store2.ListPending()
	if len(pending) != 1 {
		t.Errorf("pending count = %d, want 1", len(pending))
	}
}

func TestPairingRequest_IsExpired(t *testing.T) {
	now := time.Now()

	req := &PairingRequest{
		ID:        "user1",
		Name:      "Alice",
		Code:      "ABC12345",
		CreatedAt: now.Add(-time.Hour).Unix(),
		ExpiresAt: now.Add(-time.Minute).Unix(), // Expired 1 minute ago
	}

	if !req.IsExpired() {
		t.Error("request should be expired")
	}

	req.ExpiresAt = now.Add(time.Hour).Unix() // Expires in 1 hour
	if req.IsExpired() {
		t.Error("request should not be expired")
	}
}

func TestBuildPairingReply(t *testing.T) {
	reply := BuildPairingReply("feishu", "Your Feishu user id: ou_xxx", "ABCD1234")

	expected := "👋 Hi! Your pairing code for feishu is:\n\nABCD1234\n\nYour Feishu user id: ou_xxx\n\n" +
		"To approve, run: anyclaw pairing approve feishu ABCD1234"

	if reply != expected {
		t.Errorf("reply = %s, want %s", reply, expected)
	}
}
