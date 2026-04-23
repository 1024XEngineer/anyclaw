package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestScanReferences(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"db_url": {
			Key:   "db_url",
			Value: "postgresql://${SECRET:db_host}:${SECRET:db_port}/mydb",
			Scope: ScopeGlobal,
		},
		"db_host": {
			Key:   "db_host",
			Value: "localhost",
			Scope: ScopeGlobal,
		},
		"db_port": {
			Key:   "db_port",
			Value: "5432",
			Scope: ScopeGlobal,
		},
		"api_key": {
			Key:   "api_key",
			Value: "plain-key",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	refs := snap.ScanReferences()

	if len(refs) != 1 {
		t.Fatalf("expected 1 entry with references, got %d", len(refs))
	}

	dbRefs := refs["db_url"]
	if len(dbRefs) != 2 {
		t.Fatalf("expected 2 refs for db_url, got %d", len(dbRefs))
	}
	if dbRefs[0] != "db_host" || dbRefs[1] != "db_port" {
		t.Errorf("unexpected refs: %v", dbRefs)
	}
}

func TestValidateReferencesStrict(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"db_url": {
			Key:   "db_url",
			Value: "postgresql://${SECRET:db_host}:5432/mydb",
			Scope: ScopeGlobal,
		},
		"db_host": {
			Key:   "db_host",
			Value: "localhost",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	result := snap.ValidateReferences(ValidationStrict, nil)

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
	if result.Scanned != 1 {
		t.Errorf("expected 1 scanned entry, got %d", result.Scanned)
	}
	if len(result.Refs["db_url"]) != 1 {
		t.Errorf("expected 1 ref for db_url, got %d", len(result.Refs["db_url"]))
	}
}

func TestValidateReferencesMissingRef(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"db_url": {
			Key:   "db_url",
			Value: "postgresql://${SECRET:missing_host}:5432/mydb",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	result := snap.ValidateReferences(ValidationStrict, nil)

	if result.Valid {
		t.Fatal("expected validation to fail")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].RefKey != "missing_host" {
		t.Errorf("expected ref 'missing_host', got '%s'", result.Errors[0].RefKey)
	}
}

func TestValidateReferencesRequiredKeys(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key-123",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	result := snap.ValidateReferences(ValidationStrict, []string{"api_key", "db_password", "token"})

	if result.Valid {
		t.Fatal("expected validation to fail")
	}

	var missingKeys []string
	for _, err := range result.Errors {
		if err.Message == "required secret is missing" {
			missingKeys = append(missingKeys, err.SecretKey)
		}
	}

	if len(missingKeys) != 2 {
		t.Fatalf("expected 2 missing required keys, got %d: %v", len(missingKeys), missingKeys)
	}
}

func TestValidateReferencesEmptyValue(t *testing.T) {
	secrets := map[string]*SecretEntry{
		"empty_key": {
			Key:   "empty_key",
			Value: "",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")

	result := snap.ValidateReferences(ValidationStrict, nil)
	if result.Valid {
		t.Fatal("expected strict validation to fail on empty value")
	}

	result = snap.ValidateReferences(ValidationWarn, nil)
	if !result.Valid {
		t.Fatal("expected warn mode to pass on empty value")
	}
}

func TestResolveValueStrict(t *testing.T) {
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

	resolved, err := snap.ResolveValueStrict("postgresql://${SECRET:host}:${SECRET:port}/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != "postgresql://localhost:5432/mydb" {
		t.Errorf("expected 'postgresql://localhost:5432/mydb', got '%s'", resolved)
	}

	_, err = snap.ResolveValueStrict("postgresql://${SECRET:missing_host}:5432/mydb")
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	if !strings.Contains(err.Error(), "missing_host") {
		t.Errorf("expected error about 'missing_host', got: %v", err)
	}
}

func TestValidateStartupFailFast(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"db_url": {
			Key:   "db_url",
			Value: "${SECRET:missing_key}",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	manager := NewActivationManager(store, snap)

	cfg := &StartupConfig{
		FailFast:       true,
		ValidationMode: ValidationStrict,
	}

	err := manager.ValidateStartup(cfg)
	if err == nil {
		t.Fatal("expected startup validation to fail")
	}
	if !strings.Contains(err.Error(), "startup validation failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateStartupWarnMode(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"db_url": {
			Key:   "db_url",
			Value: "${SECRET:missing_key}",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	manager := NewActivationManager(store, snap)

	cfg := &StartupConfig{
		FailFast:       false,
		ValidationMode: ValidationStrict,
	}

	err := manager.ValidateStartup(cfg)
	if err != nil {
		t.Fatalf("expected no error in warn mode, got: %v", err)
	}
}

func TestValidateStartupOff(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(nil, "test")
	manager := NewActivationManager(store, snap)

	cfg := &StartupConfig{
		FailFast:       true,
		ValidationMode: ValidationOff,
	}

	err := manager.ValidateStartup(cfg)
	if err != nil {
		t.Fatalf("expected no error when validation is off, got: %v", err)
	}
}

func TestValidateStartupRequiredKeys(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key-123",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "test")
	manager := NewActivationManager(store, snap)

	cfg := &StartupConfig{
		FailFast:       true,
		ValidationMode: ValidationStrict,
		RequiredKeys:   []string{"api_key", "db_password"},
	}

	err := manager.ValidateStartup(cfg)
	if err == nil {
		t.Fatal("expected error for missing required key")
	}
	if !strings.Contains(err.Error(), "db_password") {
		t.Errorf("expected error about 'db_password', got: %v", err)
	}
}

func TestValidateStartupNoSnapshot(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	manager := NewActivationManager(store, nil)

	cfg := DefaultStartupConfig()
	cfg.FailFast = true

	err := manager.ValidateStartup(cfg)
	if err == nil {
		t.Fatal("expected error when no snapshot loaded")
	}
	if !strings.Contains(err.Error(), "no active snapshot") {
		t.Errorf("expected 'no active snapshot' error, got: %v", err)
	}
}

func TestValidateStartupNoSnapshotWarn(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	manager := NewActivationManager(store, nil)

	cfg := &StartupConfig{
		FailFast:       false,
		ValidationMode: ValidationStrict,
	}

	err := manager.ValidateStartup(cfg)
	if err != nil {
		t.Fatalf("expected no error in warn mode without snapshot, got: %v", err)
	}
}

func TestResolveValueStrictNoRefs(t *testing.T) {
	snap := NewRuntimeSnapshot(map[string]*SecretEntry{}, "test")

	resolved, err := snap.ResolveValueStrict("plain-text-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != "plain-text-value" {
		t.Errorf("expected 'plain-text-value', got '%s'", resolved)
	}
}

func TestResolveValueStrictMalformed(t *testing.T) {
	snap := NewRuntimeSnapshot(map[string]*SecretEntry{}, "test")

	_, err := snap.ResolveValueStrict("${SECRET:unclosed_ref")
	if err == nil {
		t.Fatal("expected error for malformed ref")
	}
}

func TestFallbackFromLastSnapshot(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "original-key",
			Scope: ScopeGlobal,
		},
		"db_pass": {
			Key:   "db_pass",
			Value: "original-pass",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "initial")
	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackLastSnapshot

	manager := NewActivationManagerWithFallback(store, snap, fbCfg)

	manager.CreateSnapshot("pre-fallback")

	active := manager.GetActiveSnapshot()
	entry, _ := active.Get("api_key")
	if entry.Value != "original-key" {
		t.Errorf("expected 'original-key', got '%s'", entry.Value)
	}

	recovered, err := manager.DetectAndFallback("test_trigger")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}

	rEntry, ok := recovered.Get("api_key")
	if !ok {
		t.Fatal("expected api_key in recovered snapshot")
	}
	if rEntry.Value != "original-key" {
		t.Errorf("expected 'original-key' from fallback, got '%s'", rEntry.Value)
	}

	status := manager.RecoveryStatus()
	if status.State != RecoveryRecovered {
		t.Errorf("expected state recovered, got %s", status.State)
	}
	if status.AttemptCount != 1 {
		t.Errorf("expected 1 attempt, got %d", status.AttemptCount)
	}
}

func TestFallbackFromEnvVars(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	os.Setenv("ANYCLAW_SECRET_TEST_KEY", "env-secret-value")
	os.Setenv("ANYCLAW_SECRET_DB_HOST", "env-db-host")
	defer func() {
		os.Unsetenv("ANYCLAW_SECRET_TEST_KEY")
		os.Unsetenv("ANYCLAW_SECRET_DB_HOST")
	}()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "old-key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackEnvVars

	manager := NewActivationManagerWithFallback(store, snap, fbCfg)
	manager.CreateSnapshot("pre-fallback")

	recovered, err := manager.DetectAndFallback("env_fallback_test")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}

	entry, ok := recovered.Get("TEST_KEY")
	if !ok {
		t.Fatal("expected TEST_KEY in recovered snapshot")
	}
	if entry.Value != "env-secret-value" {
		t.Errorf("expected 'env-secret-value', got '%s'", entry.Value)
	}

	entry, ok = recovered.Get("DB_HOST")
	if !ok {
		t.Fatal("expected DB_HOST in recovered snapshot")
	}
	if entry.Value != "env-db-host" {
		t.Errorf("expected 'env-db-host', got '%s'", entry.Value)
	}
}

func TestFallbackFromDefaults(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "real-key",
			Scope: ScopeGlobal,
		},
		"db_pass": {
			Key:   "db_pass",
			Value: "real-pass",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "initial")

	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackDefaults
	fbCfg.DefaultValue = "DEFAULT_PLACEHOLDER"

	manager := NewActivationManagerWithFallback(store, snap, fbCfg)
	manager.CreateSnapshot("pre-fallback")

	recovered, err := manager.DetectAndFallback("default_fallback_test")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}

	entry, ok := recovered.Get("api_key")
	if !ok {
		t.Fatal("expected api_key in recovered snapshot")
	}
	if entry.Value != "DEFAULT_PLACEHOLDER" {
		t.Errorf("expected 'DEFAULT_PLACEHOLDER', got '%s'", entry.Value)
	}

	entry, ok = recovered.Get("db_pass")
	if !ok {
		t.Fatal("expected db_pass in recovered snapshot")
	}
	if entry.Value != "DEFAULT_PLACEHOLDER" {
		t.Errorf("expected 'DEFAULT_PLACEHOLDER', got '%s'", entry.Value)
	}
}

func TestFallbackToEmpty(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "real-key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackEmpty

	manager := NewActivationManagerWithFallback(store, snap, fbCfg)
	manager.CreateSnapshot("pre-fallback")

	recovered, err := manager.DetectAndFallback("empty_fallback_test")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}

	all := recovered.GetAll()
	if len(all) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(all))
	}
}

func TestFallbackDisabled(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	fbCfg := DefaultFallbackConfig()
	fbCfg.Enabled = false

	manager := NewActivationManagerWithFallback(store, snap, fbCfg)

	_, err := manager.DetectAndFallback("test")
	if err == nil {
		t.Fatal("expected error when fallback disabled")
	}
}

func TestFallbackMaxAttempts(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackLastSnapshot
	fbCfg.MaxAttempts = 2

	manager := NewActivationManagerWithFallback(store, nil, fbCfg)

	_, err1 := manager.DetectAndFallback("attempt1")
	if err1 == nil {
		t.Fatal("expected first fallback to fail (no snapshots)")
	}

	_, err2 := manager.DetectAndFallback("attempt2")
	if err2 == nil {
		t.Fatal("expected second fallback to fail")
	}

	_, err3 := manager.DetectAndFallback("attempt3")
	if err3 == nil {
		t.Fatal("expected third fallback to be blocked by max attempts")
	}
	if !strings.Contains(err3.Error(), "max fallback attempts") {
		t.Errorf("expected max attempts error, got: %v", err3)
	}
}

func TestRestoreFromFallback(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "original",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "initial")
	fbCfg := DefaultFallbackConfig()
	manager := NewActivationManagerWithFallback(store, snap, fbCfg)
	manager.CreateSnapshot("pre-fallback")

	active := manager.GetActiveSnapshot()
	active.Update(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "corrupted",
			Scope: ScopeGlobal,
		},
	})

	cEntry, _ := active.Get("api_key")
	if cEntry.Value != "corrupted" {
		t.Errorf("expected 'corrupted', got '%s'", cEntry.Value)
	}

	if err := manager.RestoreFromFallback(); err != nil {
		t.Fatalf("RestoreFromFallback failed: %v", err)
	}

	restored := manager.GetActiveSnapshot()
	rEntry, ok := restored.Get("api_key")
	if !ok {
		t.Fatal("expected api_key after restore")
	}
	if rEntry.Value != "original" {
		t.Errorf("expected 'original' after restore, got '%s'", rEntry.Value)
	}
}

func TestResetRecovery(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackLastSnapshot

	manager := NewActivationManagerWithFallback(store, snap, fbCfg)
	manager.CreateSnapshot("pre-fallback")

	manager.DetectAndFallback("test")

	status := manager.RecoveryStatus()
	if status.State != RecoveryRecovered {
		t.Fatalf("expected recovered state, got %s", status.State)
	}
	if status.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", status.AttemptCount)
	}

	manager.ResetRecovery()

	status = manager.RecoveryStatus()
	if status.State != RecoveryNormal {
		t.Errorf("expected normal state after reset, got %s", status.State)
	}
	if status.AttemptCount != 0 {
		t.Errorf("expected 0 attempts after reset, got %d", status.AttemptCount)
	}
}

func TestRecoveryStatus(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "initial")
	fbCfg := DefaultFallbackConfig()
	manager := NewActivationManagerWithFallback(store, snap, fbCfg)

	status := manager.RecoveryStatus()
	if status.State != RecoveryNormal {
		t.Errorf("expected normal state, got %s", status.State)
	}

	manager.DetectAndFallback("test")
	status = manager.RecoveryStatus()
	if status.State != RecoveryRecovered {
		t.Errorf("expected recovered state, got %s", status.State)
	}
	if status.LastEvent == nil {
		t.Fatal("expected last event to be set")
	}
	if !status.LastEvent.Success {
		t.Error("expected last event to be successful")
	}
}

func TestFallbackWithStoreSnapshot(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	store.SetSecret(&SecretEntry{
		Key:   "stored_key",
		Value: "stored-value",
		Scope: ScopeGlobal,
	})
	store.CreateSnapshot("stored")

	fbCfg := DefaultFallbackConfig()
	fbCfg.Strategy = FallbackLastSnapshot

	manager := NewActivationManagerWithFallback(store, nil, fbCfg)

	recovered, err := manager.DetectAndFallback("store_snapshot_test")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}

	entry, ok := recovered.Get("stored_key")
	if !ok {
		t.Fatal("expected stored_key from store snapshot")
	}
	if entry.Value != "stored-value" {
		t.Errorf("expected 'stored-value', got '%s'", entry.Value)
	}
}

func TestRotateSecret(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	secrets := map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "old-key-value",
			Scope: ScopeGlobal,
		},
	}

	snap := NewRuntimeSnapshot(secrets, "initial")
	fbCfg := DefaultFallbackConfig()
	manager := NewActivationManagerWithFallback(store, snap, fbCfg)

	req := &RotationRequest{
		Key:         "api_key",
		NewValue:    "new-key-value",
		RequestedBy: "admin",
		Reason:      "quarterly rotation",
		ActivateNow: true,
	}

	result, err := manager.RotateSecret(req)
	if err != nil {
		t.Fatalf("RotateSecret failed: %v", err)
	}
	if result.Key != "api_key" {
		t.Errorf("expected key api_key, got %s", result.Key)
	}
	if result.OldVersion != 0 {
		t.Errorf("expected old version 0, got %d", result.OldVersion)
	}
	if result.NewVersion != 1 {
		t.Errorf("expected new version 1, got %d", result.NewVersion)
	}
	if !result.Activated {
		t.Error("expected activated to be true")
	}

	active := manager.GetActiveSnapshot()
	entry, ok := active.Get("api_key")
	if !ok {
		t.Fatal("expected api_key in active snapshot")
	}
	if entry.Value != "new-key-value" {
		t.Errorf("expected 'new-key-value', got '%s'", entry.Value)
	}
	if entry.Metadata["version"] != "1" {
		t.Errorf("expected version metadata '1', got '%s'", entry.Metadata["version"])
	}
}

func TestRotateSecretMultipleTimes(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"db_pass": {
			Key:   "db_pass",
			Value: "pass-v1",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	for i := 1; i <= 3; i++ {
		req := &RotationRequest{
			Key:         "db_pass",
			NewValue:    fmt.Sprintf("pass-v%d", i+1),
			RequestedBy: "admin",
			Reason:      fmt.Sprintf("rotation %d", i),
			ActivateNow: true,
		}
		result, err := manager.RotateSecret(req)
		if err != nil {
			t.Fatalf("rotation %d failed: %v", i, err)
		}
		if result.NewVersion != uint64(i) {
			t.Errorf("rotation %d: expected version %d, got %d", i, i, result.NewVersion)
		}
	}

	vh, err := manager.GetVersionHistory("db_pass")
	if err != nil {
		t.Fatalf("GetVersionHistory failed: %v", err)
	}
	if vh.Current != 3 {
		t.Errorf("expected current version 3, got %d", vh.Current)
	}
	if vh.TotalRotates != 3 {
		t.Errorf("expected 3 total rotates, got %d", vh.TotalRotates)
	}
	if len(vh.Versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(vh.Versions))
	}
}

func TestVersionHistory(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"token": {
			Key:   "token",
			Value: "v1",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	manager.RotateSecret(&RotationRequest{
		Key:         "token",
		NewValue:    "v2",
		RequestedBy: "user1",
		Reason:      "rotate",
		ActivateNow: true,
	})
	manager.RotateSecret(&RotationRequest{
		Key:         "token",
		NewValue:    "v3",
		RequestedBy: "user2",
		Reason:      "rotate again",
		ActivateNow: true,
	})

	vh, err := manager.GetVersionHistory("token")
	if err != nil {
		t.Fatalf("GetVersionHistory failed: %v", err)
	}

	if vh.Key != "token" {
		t.Errorf("expected key 'token', got '%s'", vh.Key)
	}
	if vh.Current != 2 {
		t.Errorf("expected current 2, got %d", vh.Current)
	}
	if vh.LastRotation == nil {
		t.Error("expected last rotation to be set")
	}

	activeVer := vh.Versions[len(vh.Versions)-1]
	if !activeVer.Active {
		t.Error("expected latest version to be active")
	}
	if activeVer.Value != "v3" {
		t.Errorf("expected active value 'v3', got '%s'", activeVer.Value)
	}

	inactiveVer := vh.Versions[0]
	if inactiveVer.Active {
		t.Error("expected first version to be inactive")
	}
}

func TestRollbackVersion(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "v1-value",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	manager.RotateSecret(&RotationRequest{
		Key:         "api_key",
		NewValue:    "v2-value",
		RequestedBy: "admin",
		Reason:      "rotate",
		ActivateNow: true,
	})
	manager.RotateSecret(&RotationRequest{
		Key:         "api_key",
		NewValue:    "v3-value",
		RequestedBy: "admin",
		Reason:      "rotate again",
		ActivateNow: true,
	})

	active := manager.GetActiveSnapshot()
	entry, _ := active.Get("api_key")
	if entry.Value != "v3-value" {
		t.Fatalf("expected 'v3-value' before rollback, got '%s'", entry.Value)
	}

	if err := manager.RollbackVersion("api_key", 1, "admin"); err != nil {
		t.Fatalf("RollbackVersion failed: %v", err)
	}

	active = manager.GetActiveSnapshot()
	entry, _ = active.Get("api_key")
	if entry.Value != "v2-value" {
		t.Errorf("expected 'v2-value' after rollback to v1 (version 1 is v2-value), got '%s'", entry.Value)
	}

	vh, _ := manager.GetVersionHistory("api_key")
	if vh.Current != 1 {
		t.Errorf("expected current version 1 after rollback, got %d", vh.Current)
	}
}

func TestRollbackVersionNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "v1",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	err := manager.RollbackVersion("api_key", 99, "admin")
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
}

func TestRotationPolicy(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key-value",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	policy := &RotationPolicy{
		Key:          "api_key",
		Strategy:     RotationScheduled,
		Interval:     30 * 24 * time.Hour,
		MaxVersions:  5,
		GracePeriod:  24 * time.Hour,
		AutoActivate: true,
	}

	if err := manager.SetRotationPolicy(policy); err != nil {
		t.Fatalf("SetRotationPolicy failed: %v", err)
	}

	got, ok := manager.GetRotationPolicy("api_key")
	if !ok {
		t.Fatal("expected rotation policy to exist")
	}
	if got.Strategy != RotationScheduled {
		t.Errorf("expected strategy scheduled, got %s", got.Strategy)
	}
	if got.MaxVersions != 5 {
		t.Errorf("expected max versions 5, got %d", got.MaxVersions)
	}

	policies := manager.ListRotationPolicies()
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	if err := manager.DeleteRotationPolicy("api_key"); err != nil {
		t.Fatalf("DeleteRotationPolicy failed: %v", err)
	}

	_, ok = manager.GetRotationPolicy("api_key")
	if ok {
		t.Fatal("expected policy to be deleted")
	}
}

func TestCheckScheduledRotations(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "current-key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	past := time.Now().Add(-time.Hour)
	policy := &RotationPolicy{
		Key:          "api_key",
		Strategy:     RotationScheduled,
		Interval:     24 * time.Hour,
		NextRotation: &past,
		AutoActivate: true,
	}
	manager.SetRotationPolicy(policy)

	results, err := manager.CheckScheduledRotations("system")
	if err != nil {
		t.Fatalf("CheckScheduledRotations failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 rotation result, got %d", len(results))
	}
	if results[0].Key != "api_key" {
		t.Errorf("expected key api_key, got %s", results[0].Key)
	}
	if results[0].NewVersion != 1 {
		t.Errorf("expected new version 1, got %d", results[0].NewVersion)
	}
	if results[0].Activated != true {
		t.Error("expected activated to be true")
	}
}

func TestCheckScheduledRotationsNotDue(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "current-key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	future := time.Now().Add(24 * time.Hour)
	policy := &RotationPolicy{
		Key:          "api_key",
		Strategy:     RotationScheduled,
		Interval:     24 * time.Hour,
		NextRotation: &future,
	}
	manager.SetRotationPolicy(policy)

	results, err := manager.CheckScheduledRotations("system")
	if err != nil {
		t.Fatalf("CheckScheduledRotations failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 rotations (not due yet), got %d", len(results))
	}
}

func TestCleanupOldVersions(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "v1",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	for i := 1; i <= 5; i++ {
		manager.RotateSecret(&RotationRequest{
			Key:         "api_key",
			NewValue:    fmt.Sprintf("v%d", i+1),
			RequestedBy: "admin",
			ActivateNow: true,
		})
	}

	vh, _ := manager.GetVersionHistory("api_key")
	if len(vh.Versions) != 5 {
		t.Fatalf("expected 5 versions before cleanup, got %d", len(vh.Versions))
	}

	if err := manager.CleanupOldVersions("api_key", 2); err != nil {
		t.Fatalf("CleanupOldVersions failed: %v", err)
	}

	vh, _ = manager.GetVersionHistory("api_key")
	if len(vh.Versions) != 2 {
		t.Errorf("expected 2 versions after cleanup, got %d", len(vh.Versions))
	}
}

func TestRotationPolicyMaxVersions(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "v1",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	manager.SetRotationPolicy(&RotationPolicy{
		Key:         "api_key",
		MaxVersions: 3,
	})

	for i := 1; i <= 5; i++ {
		manager.RotateSecret(&RotationRequest{
			Key:         "api_key",
			NewValue:    fmt.Sprintf("v%d", i+1),
			RequestedBy: "admin",
			ActivateNow: true,
		})
	}

	vh, _ := manager.GetVersionHistory("api_key")
	if len(vh.Versions) != 3 {
		t.Errorf("expected 3 versions (max_versions limit), got %d", len(vh.Versions))
	}
}

func TestRotateSecretNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"api_key": {
			Key:   "api_key",
			Value: "key",
			Scope: ScopeGlobal,
		},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	_, err := manager.RotateSecret(&RotationRequest{
		Key:         "nonexistent",
		NewValue:    "new-value",
		RequestedBy: "admin",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestListVersionHistories(t *testing.T) {
	store, cleanup := setupTestStore(t, nil)
	defer cleanup()

	snap := NewRuntimeSnapshot(map[string]*SecretEntry{
		"key1": {Key: "key1", Value: "v1", Scope: ScopeGlobal},
		"key2": {Key: "key2", Value: "v1", Scope: ScopeGlobal},
	}, "initial")

	manager := NewActivationManagerWithFallback(store, snap, nil)

	manager.RotateSecret(&RotationRequest{
		Key: "key1", NewValue: "v2", RequestedBy: "admin", ActivateNow: true,
	})
	manager.RotateSecret(&RotationRequest{
		Key: "key2", NewValue: "v2", RequestedBy: "admin", ActivateNow: true,
	})

	histories := manager.ListVersionHistories()
	if len(histories) != 2 {
		t.Fatalf("expected 2 version histories, got %d", len(histories))
	}
}
