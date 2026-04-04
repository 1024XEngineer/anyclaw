package secrets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestStore(t *testing.T, cfg *StoreConfig) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	if cfg == nil {
		cfg = DefaultStoreConfig()
	}
	cfg.Path = filepath.Join(dir, "anyclaw.json")

	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}
	return store, cleanup
}

func TestStoreSecretCRUD(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	entry := &SecretEntry{
		Key:      "api_key",
		Value:    "secret-123",
		Scope:    ScopeApp,
		ScopeRef: "myapp",
		Source:   SourceManual,
	}

	if err := store.SetSecret(entry); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	got, ok := store.GetSecret("api_key", ScopeApp, "myapp")
	if !ok {
		t.Fatal("expected secret to exist")
	}
	if got.Value != "secret-123" {
		t.Errorf("expected value 'secret-123', got '%s'", got.Value)
	}
	if got.ID == "" {
		t.Error("expected ID to be set")
	}

	secrets := store.ListSecrets(ScopeApp, "myapp")
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}

	if err := store.DeleteSecret("api_key", ScopeApp, "myapp"); err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	_, ok = store.GetSecret("api_key", ScopeApp, "myapp")
	if ok {
		t.Fatal("expected secret to be deleted")
	}
}

func TestStoreSecretWithExpiry(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	expired := time.Now().Add(-time.Hour)
	entry := &SecretEntry{
		Key:       "temp_token",
		Value:     "expired-value",
		Scope:     ScopeGlobal,
		ExpiresAt: &expired,
	}

	if err := store.SetSecret(entry); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	_, ok := store.GetSecret("temp_token", ScopeGlobal, "")
	if ok {
		t.Fatal("expected expired secret to not be returned")
	}
}

func TestStoreSnapshot(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	store.SetSecret(&SecretEntry{
		Key:   "key1",
		Value: "value1",
		Scope: ScopeGlobal,
	})
	store.SetSecret(&SecretEntry{
		Key:   "key2",
		Value: "value2",
		Scope: ScopeGlobal,
	})

	snap, err := store.CreateSnapshot("test")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if snap.Version != 0 {
		t.Errorf("expected version 0, got %d", snap.Version)
	}
	if len(snap.Secrets) != 2 {
		t.Errorf("expected 2 secrets in snapshot, got %d", len(snap.Secrets))
	}
	if snap.Checksum == "" {
		t.Error("expected checksum to be set")
	}

	got, ok := store.GetSnapshot(snap.ID)
	if !ok {
		t.Fatal("expected snapshot to exist")
	}
	if got.Version != snap.Version {
		t.Errorf("expected version %d, got %d", snap.Version, got.Version)
	}

	snaps := store.ListSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	store.SetSecret(&SecretEntry{
		Key:   "key3",
		Value: "value3",
		Scope: ScopeGlobal,
	})

	snap2, err := store.CreateSnapshot("test2")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if snap2.Version != 1 {
		t.Errorf("expected version 1, got %d", snap2.Version)
	}
}

func TestStoreRestoreSnapshot(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	store.SetSecret(&SecretEntry{
		Key:   "key1",
		Value: "value1",
		Scope: ScopeGlobal,
	})

	snap, err := store.CreateSnapshot("initial")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	store.SetSecret(&SecretEntry{
		Key:   "key2",
		Value: "value2",
		Scope: ScopeGlobal,
	})

	if err := store.RestoreSnapshot(snap.ID); err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	secrets := store.ListSecrets("", "")
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret after restore, got %d", len(secrets))
	}
	if secrets[0].Key != "key1" {
		t.Errorf("expected key1, got %s", secrets[0].Key)
	}
}

func TestStoreActivationLock(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	lock := &ActivationLock{
		SnapshotID:    "snap_123",
		SnapshotVer:   1,
		State:         LockPending,
		RequestedBy:   "user1",
		Reason:        "deploy to production",
		RequiresCount: 2,
	}

	if err := store.CreateLock(lock); err != nil {
		t.Fatalf("CreateLock failed: %v", err)
	}
	if lock.ID == "" {
		t.Error("expected lock ID to be set")
	}

	got, ok := store.GetLock(lock.ID)
	if !ok {
		t.Fatal("expected lock to exist")
	}
	if got.State != LockPending {
		t.Errorf("expected state pending, got %s", got.State)
	}

	locks := store.ListLocks(LockPending)
	if len(locks) != 1 {
		t.Fatalf("expected 1 pending lock, got %d", len(locks))
	}
}

func TestStoreUpdateLock(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	lock := &ActivationLock{
		SnapshotID:    "snap_123",
		State:         LockPending,
		RequestedBy:   "user1",
		RequiresCount: 1,
	}
	store.CreateLock(lock)

	err := store.UpdateLock(lock.ID, func(l *ActivationLock) error {
		l.State = LockActivated
		now := time.Now().UTC()
		l.ActivatedAt = &now
		l.ActivatedBy = "approver1"
		l.Approvals = append(l.Approvals, LockApproval{
			Approver:   "approver1",
			ApprovedAt: now,
		})
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateLock failed: %v", err)
	}

	got, ok := store.GetLock(lock.ID)
	if !ok {
		t.Fatal("expected lock to exist")
	}
	if got.State != LockActivated {
		t.Errorf("expected state activated, got %s", got.State)
	}
	if len(got.Approvals) != 1 {
		t.Errorf("expected 1 approval, got %d", len(got.Approvals))
	}
}

func TestStoreAuditLog(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	entry := &AuditEntry{
		Operation: OpCreate,
		SecretKey: "api_key",
		Actor:     "user1",
		Success:   true,
	}

	if err := store.AddAuditEntry(entry); err != nil {
		t.Fatalf("AddAuditEntry failed: %v", err)
	}

	entries := store.ListAuditEntries(10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Operation != OpCreate {
		t.Errorf("expected operation create, got %s", entries[0].Operation)
	}
}

func TestEncryptionDecryption(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey failed: %v", err)
	}

	keyBytes, err := decodeKey(key)
	if err != nil {
		t.Fatalf("decodeKey failed: %v", err)
	}
	if len(keyBytes) != 32 {
		t.Errorf("expected 32 byte key, got %d", len(keyBytes))
	}

	plaintext := "my-super-secret-value"
	encrypted, err := EncryptValue(plaintext, keyBytes)
	if err != nil {
		t.Fatalf("EncryptValue failed: %v", err)
	}
	if encrypted == plaintext {
		t.Error("expected encrypted value to differ from plaintext")
	}

	decrypted, err := DecryptValue(encrypted, keyBytes)
	if err != nil {
		t.Fatalf("DecryptValue failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("expected '%s', got '%s'", plaintext, decrypted)
	}
}

func TestStoreWithEncryption(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey failed: %v", err)
	}

	cfg := DefaultStoreConfig()
	cfg.EncryptionKey = key

	store, cleanup := setupTestStore(t, cfg)
	defer cleanup()

	entry := &SecretEntry{
		Key:   "encrypted_key",
		Value: "secret-value",
		Scope: ScopeGlobal,
	}

	if err := store.SetSecret(entry); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	got, ok := store.GetSecret("encrypted_key", ScopeGlobal, "")
	if !ok {
		t.Fatal("expected secret to exist")
	}
	if got.Value != "secret-value" {
		t.Errorf("expected 'secret-value', got '%s'", got.Value)
	}
}

func TestRuntimeSnapshot(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"db_password": {
			Key:   "db_password",
			Value: "super-secret",
			Scope: ScopeGlobal,
		},
		"api_key": {
			Key:   "api_key",
			Value: "key-123",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")

	if snap.Version() != 1 {
		t.Errorf("expected version 1, got %d", snap.Version())
	}

	entry, ok := snap.Get("db_password")
	if !ok {
		t.Fatal("expected db_password to exist")
	}
	if entry.Value != "super-secret" {
		t.Errorf("expected 'super-secret', got '%s'", entry.Value)
	}

	_, ok = snap.Get("nonexistent")
	if ok {
		t.Fatal("expected nonexistent key to not exist")
	}

	all := snap.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(all))
	}
}

func TestRuntimeSnapshotResolveValue(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"host": {
			Key:   "host",
			Value: "localhost",
			Scope: ScopeGlobal,
		},
		"port": {
			Key:   "port",
			Value: "5432",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")

	template := "postgresql://${SECRET:host}:${SECRET:port}/mydb"
	resolved := snap.ResolveValue(template)

	expected := "postgresql://localhost:5432/mydb"
	if resolved != expected {
		t.Errorf("expected '%s', got '%s'", expected, resolved)
	}

	template2 := "no references here"
	if snap.ResolveValue(template2) != template2 {
		t.Error("expected template without references to be unchanged")
	}
}

func TestRuntimeSnapshotRedact(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"token": {
			Key:   "token",
			Value: "secret-token-value",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")

	text := "The token is secret-token-value and should be redacted"
	redacted := snap.Redact(text)

	expected := "The token is [REDACTED:token] and should be redacted"
	if redacted != expected {
		t.Errorf("expected '%s', got '%s'", expected, redacted)
	}
}

func TestRuntimeSnapshotUpdate(t *testing.T) {
	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "value1", Scope: ScopeGlobal},
	}, "test")

	if snap.Version() != 1 {
		t.Errorf("expected version 1, got %d", snap.Version())
	}

	oldChecksum := snap.Checksum()

	snap.Update(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "new-value1", Scope: ScopeGlobal},
		"key2": {Key: "key2", Value: "value2", Scope: ScopeGlobal},
	})

	if snap.Version() != 2 {
		t.Errorf("expected version 2, got %d", snap.Version())
	}

	if snap.Checksum() == oldChecksum {
		t.Error("expected checksum to change after update")
	}

	entry, ok := snap.Get("key2")
	if !ok {
		t.Fatal("expected key2 to exist")
	}
	if entry.Value != "value2" {
		t.Errorf("expected 'value2', got '%s'", entry.Value)
	}
}

func TestActivationManagerRequestActivation(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "value1", Scope: ScopeGlobal},
	}, "initial")

	manager := NewActivationManager(store, snap)

	lock, err := manager.RequestActivation("user1", "deploy to prod")
	if err != nil {
		t.Fatalf("RequestActivation failed: %v", err)
	}
	if lock.State != LockPending {
		t.Errorf("expected state pending, got %s", lock.State)
	}
	if lock.RequestedBy != "user1" {
		t.Errorf("expected requestedBy 'user1', got '%s'", lock.RequestedBy)
	}

	if !manager.IsLocked() {
		t.Error("expected manager to be locked")
	}
}

func TestActivationManagerApprove(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "value1", Scope: ScopeGlobal},
	}, "initial")

	manager := NewActivationManager(store, snap)
	lock, _ := manager.RequestActivation("user1", "deploy")

	approved, err := manager.Approve(lock.ID, "approver1", "looks good")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	if approved.State != LockActivated {
		t.Errorf("expected state activated, got %s", approved.State)
	}
	if len(approved.Approvals) != 1 {
		t.Errorf("expected 1 approval, got %d", len(approved.Approvals))
	}
}

func TestActivationManagerRevoke(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "value1", Scope: ScopeGlobal},
	}, "initial")

	manager := NewActivationManager(store, snap)
	lock, _ := manager.RequestActivation("user1", "deploy")

	if err := manager.Revoke(lock.ID, "admin1", "cancelled"); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	got := manager.GetActiveLock()
	if got.State != LockRevoked {
		t.Errorf("expected state revoked, got %s", got.State)
	}
}

func TestActivationManagerApplySnapshot(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap1 := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "value1", Scope: ScopeGlobal},
	}, "v1")

	manager := NewActivationManager(store, snap1)

	snap2 := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "new-value1", Scope: ScopeGlobal},
		"key2": {Key: "key2", Value: "value2", Scope: ScopeGlobal},
	}, "v2")

	if err := manager.ApplySnapshot(snap2, "user1"); err != nil {
		t.Fatalf("ApplySnapshot failed: %v", err)
	}

	active := manager.GetActiveSnapshot()
	entry, ok := active.Get("key2")
	if !ok {
		t.Fatal("expected key2 to exist in new snapshot")
	}
	if entry.Value != "value2" {
		t.Errorf("expected 'value2', got '%s'", entry.Value)
	}
}

func TestActivationManagerAccessSecret(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {Key: "api_key", Value: "secret-key", Scope: ScopeGlobal},
	}, "test")

	manager := NewActivationManager(store, snap)

	entry, err := manager.AccessSecret("api_key", "user1")
	if err != nil {
		t.Fatalf("AccessSecret failed: %v", err)
	}
	if entry.Value != "secret-key" {
		t.Errorf("expected 'secret-key', got '%s'", entry.Value)
	}

	_, err = manager.AccessSecret("nonexistent", "user1")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestActivationManagerStatus(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "value1", Scope: ScopeGlobal},
	}, "test")

	manager := NewActivationManager(store, snap)

	status := manager.Status()

	if status["has_active_snapshot"] != true {
		t.Error("expected has_active_snapshot to be true")
	}
	if status["snapshot_version"] != uint64(1) {
		t.Errorf("expected snapshot_version 1, got %v", status["snapshot_version"])
	}
	if status["is_locked"] != false {
		t.Error("expected is_locked to be false")
	}

	manager.RequestActivation("user1", "deploy")
	status = manager.Status()

	if status["is_locked"] != true {
		t.Error("expected is_locked to be true after request")
	}
	if status["lock_state"] != string(LockPending) {
		t.Errorf("expected lock_state pending, got %v", status["lock_state"])
	}
}

func TestAuditReporter(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	store.AddAuditEntry(&AuditEntry{
		Operation: OpCreate,
		SecretKey: "key1",
		Actor:     "user1",
		Success:   true,
	})
	store.AddAuditEntry(&AuditEntry{
		Operation: OpAccess,
		SecretKey: "key1",
		Actor:     "user2",
		Success:   true,
	})
	store.AddAuditEntry(&AuditEntry{
		Operation: OpDelete,
		SecretKey: "key2",
		Actor:     "user1",
		Success:   false,
		Error:     "permission denied",
	})

	reporter := NewAuditReporter(store)

	entries := reporter.Query(&AuditQuery{
		Actor: "user1",
	})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for user1, got %d", len(entries))
	}

	accessHistory := reporter.SecretAccessHistory("key1", 10)
	if len(accessHistory) != 1 {
		t.Errorf("expected 1 access entry for key1, got %d", len(accessHistory))
	}

	summary := reporter.Summary(time.Time{})
	if summary.TotalOperations != 3 {
		t.Errorf("expected 3 total operations, got %d", summary.TotalOperations)
	}
	if summary.RecentFailures != 1 {
		t.Errorf("expected 1 recent failure, got %d", summary.RecentFailures)
	}

	failed := reporter.FailedOperations(time.Time{}, 10)
	if len(failed) != 1 {
		t.Errorf("expected 1 failed operation, got %d", len(failed))
	}
}

func TestAuditReporterExportCSV(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	store.AddAuditEntry(&AuditEntry{
		Operation: OpCreate,
		SecretKey: "key1",
		Actor:     "user1",
		Success:   true,
		IP:        "127.0.0.1",
	})

	reporter := NewAuditReporter(store)
	entries := reporter.Query(nil)

	csv := reporter.ExportCSV(entries)

	if csv == "" {
		t.Fatal("expected CSV output")
	}
	if !containsString(csv, "timestamp,operation,secret_key") {
		t.Error("expected CSV header")
	}
	if !containsString(csv, "user1") {
		t.Error("expected user1 in CSV")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestIsEncryptedValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"short value", "abc", false},
		{"plain text", "not-encrypted-value", false},
		{"base64 but short", "YWJj", false},
		{"valid encrypted value", "aW5pdGlhbGl6YXRpb25WZWN0b3IxMjM0NTY3ODkwYWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo=", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEncryptedValue(tt.value)
			if got != tt.want {
				t.Errorf("isEncryptedValue(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
