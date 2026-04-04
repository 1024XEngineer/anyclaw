package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type RuntimeSnapshot struct {
	mu        sync.RWMutex
	version   uint64
	secrets   map[string]*SecretEntry
	createdAt time.Time
	source    string
	checksum  string
}

func NewRuntimeSnapshot(secrets map[string]*SecretEntry, source string) *RuntimeSnapshot {
	rs := &RuntimeSnapshot{
		secrets:   make(map[string]*SecretEntry),
		createdAt: time.Now().UTC(),
		source:    source,
	}
	for k, v := range secrets {
		rs.secrets[k] = v
	}
	rs.version = 1
	rs.checksum = computeRuntimeChecksum(rs.secrets)
	return rs
}

func (rs *RuntimeSnapshot) Version() uint64 {
	return atomic.LoadUint64(&rs.version)
}

func (rs *RuntimeSnapshot) Get(key string) (*SecretEntry, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	entry, ok := rs.secrets[key]
	if !ok {
		return nil, false
	}
	if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
		return nil, false
	}
	return entry, true
}

func (rs *RuntimeSnapshot) GetAll() map[string]*SecretEntry {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	result := make(map[string]*SecretEntry)
	for k, v := range rs.secrets {
		if v.ExpiresAt == nil || time.Now().Before(*v.ExpiresAt) {
			result[k] = v
		}
	}
	return result
}

func (rs *RuntimeSnapshot) ResolveValue(template string) string {
	if !strings.Contains(template, "${SECRET:") {
		return template
	}

	result := template
	for strings.Contains(result, "${SECRET:") {
		start := strings.Index(result, "${SECRET:")
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		key := result[start+9 : end]
		if entry, ok := rs.Get(key); ok {
			result = result[:start] + entry.Value + result[end+1:]
		} else {
			result = result[:start] + result[end+1:]
		}
	}
	return result
}

func (rs *RuntimeSnapshot) Redact(text string) string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	result := text
	for _, entry := range rs.secrets {
		if len(entry.Value) > 4 {
			result = strings.ReplaceAll(result, entry.Value, "[REDACTED:"+entry.Key+"]")
		}
	}
	return result
}

func (rs *RuntimeSnapshot) Update(secrets map[string]*SecretEntry) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.secrets = make(map[string]*SecretEntry)
	for k, v := range secrets {
		rs.secrets[k] = v
	}
	atomic.AddUint64(&rs.version, 1)
	rs.createdAt = time.Now().UTC()
	rs.checksum = computeRuntimeChecksum(rs.secrets)
}

func (rs *RuntimeSnapshot) Checksum() string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.checksum
}

func (rs *RuntimeSnapshot) ToSnapshot() *Snapshot {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	secrets := make(map[string]*SecretEntry)
	for k, v := range rs.secrets {
		secrets[k] = v
	}

	return &Snapshot{
		ID:        fmt.Sprintf("runtime_%d", rs.createdAt.UnixNano()),
		Version:   atomic.LoadUint64(&rs.version),
		Secrets:   secrets,
		CreatedAt: rs.createdAt,
		Source:    rs.source,
		Checksum:  rs.checksum,
	}
}

type ActivationManager struct {
	mu         sync.RWMutex
	store      *Store
	activeLock *ActivationLock
	activeSnap *RuntimeSnapshot
	approvals  map[string]bool
	config     *StoreConfig
}

func NewActivationManager(store *Store, initialSnap *RuntimeSnapshot) *ActivationManager {
	return &ActivationManager{
		store:      store,
		activeSnap: initialSnap,
		approvals:  make(map[string]bool),
		config:     store.Config(),
	}
}

func (am *ActivationManager) GetActiveSnapshot() *RuntimeSnapshot {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.activeSnap
}

func (am *ActivationManager) GetActiveLock() *ActivationLock {
	am.mu.RLock()
	defer am.mu.RUnlock()
	if am.activeLock == nil {
		return nil
	}
	return cloneLock(am.activeLock)
}

func (am *ActivationManager) IsLocked() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.activeLock != nil &&
		(am.activeLock.State == LockPending || am.activeLock.State == LockActivated) &&
		(am.activeLock.ExpiresAt == nil || time.Now().Before(*am.activeLock.ExpiresAt))
}

func (am *ActivationManager) RequestActivation(requestedBy, reason string) (*ActivationLock, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.activeLock != nil &&
		(am.activeLock.State == LockPending || am.activeLock.State == LockActivated) &&
		(am.activeLock.ExpiresAt == nil || time.Now().Before(*am.activeLock.ExpiresAt)) {
		return nil, fmt.Errorf("activation already pending")
	}

	snap := am.activeSnap.ToSnapshot()

	lock := &ActivationLock{
		SnapshotID:    snap.ID,
		SnapshotVer:   snap.Version,
		State:         LockPending,
		RequestedBy:   requestedBy,
		Reason:        reason,
		RequiresCount: am.config.ApprovalCount,
		Approvals:     []LockApproval{},
	}

	if err := am.store.CreateLock(lock); err != nil {
		return nil, fmt.Errorf("create lock: %w", err)
	}

	am.activeLock = cloneLock(lock)
	am.approvals = make(map[string]bool)

	if err := am.store.AddAuditEntry(&AuditEntry{
		Operation:  OpActivate,
		SnapshotID: snap.ID,
		LockID:     lock.ID,
		Actor:      requestedBy,
		Details:    map[string]interface{}{"reason": reason, "state": "requested"},
		Success:    true,
	}); err != nil {
		return nil, err
	}

	return cloneLock(lock), nil
}

func (am *ActivationManager) Approve(lockID, approver, comment string) (*ActivationLock, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.activeLock == nil || am.activeLock.ID != lockID {
		return nil, fmt.Errorf("lock not found")
	}

	if am.activeLock.State != LockPending {
		return nil, fmt.Errorf("lock is not pending")
	}

	if am.activeLock.ExpiresAt != nil && time.Now().After(*am.activeLock.ExpiresAt) {
		am.activeLock.State = LockExpired
		return nil, fmt.Errorf("lock expired")
	}

	if am.approvals[approver] {
		return nil, fmt.Errorf("already approved")
	}

	approval := LockApproval{
		Approver:   approver,
		ApprovedAt: time.Now().UTC(),
		Comment:    comment,
	}
	am.activeLock.Approvals = append(am.activeLock.Approvals, approval)
	am.approvals[approver] = true

	if len(am.activeLock.Approvals) >= am.activeLock.RequiresCount {
		am.activeLock.State = LockActivated
		now := time.Now().UTC()
		am.activeLock.ActivatedBy = approver
		am.activeLock.ActivatedAt = &now

		if err := am.store.AddAuditEntry(&AuditEntry{
			Operation: OpActivate,
			LockID:    lockID,
			Actor:     approver,
			Details:   map[string]interface{}{"state": "activated", "approvals": len(am.activeLock.Approvals)},
			Success:   true,
		}); err != nil {
			return nil, err
		}
	}

	if err := am.store.UpdateLock(lockID, func(l *ActivationLock) error {
		l.State = am.activeLock.State
		l.Approvals = am.activeLock.Approvals
		l.ActivatedBy = am.activeLock.ActivatedBy
		l.ActivatedAt = am.activeLock.ActivatedAt
		return nil
	}); err != nil {
		return nil, err
	}

	return cloneLock(am.activeLock), nil
}

func (am *ActivationManager) Revoke(lockID, revokedBy, reason string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.activeLock == nil || am.activeLock.ID != lockID {
		return fmt.Errorf("lock not found")
	}

	now := time.Now().UTC()
	am.activeLock.State = LockRevoked
	am.activeLock.RevokedBy = revokedBy
	am.activeLock.RevokedAt = &now

	if err := am.store.UpdateLock(lockID, func(l *ActivationLock) error {
		l.State = LockRevoked
		l.RevokedBy = revokedBy
		l.RevokedAt = &now
		return nil
	}); err != nil {
		return err
	}

	return am.store.AddAuditEntry(&AuditEntry{
		Operation: OpRevoke,
		LockID:    lockID,
		Actor:     revokedBy,
		Details:   map[string]interface{}{"reason": reason},
		Success:   true,
	})
}

func (am *ActivationManager) ApplySnapshot(newSnap *RuntimeSnapshot, actor string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.config.RequireApproval && am.activeLock != nil && am.activeLock.State != LockActivated {
		return fmt.Errorf("activation lock must be approved before applying new snapshot")
	}

	oldChecksum := ""
	if am.activeSnap != nil {
		oldChecksum = am.activeSnap.Checksum()
	}

	am.activeSnap = newSnap

	if am.activeLock != nil && am.activeLock.State == LockActivated {
		now := time.Now().UTC()
		am.activeLock.State = LockUnlocked
		am.activeLock.ActivatedAt = &now
		am.activeLock = nil
		am.approvals = make(map[string]bool)
	}

	return am.store.AddAuditEntry(&AuditEntry{
		Operation:  OpSnapshot,
		SnapshotID: newSnap.ToSnapshot().ID,
		Actor:      actor,
		Details: map[string]interface{}{
			"old_checksum": oldChecksum,
			"new_checksum": newSnap.Checksum(),
		},
		Success: true,
	})
}

func (am *ActivationManager) AccessSecret(key string, actor string) (*SecretEntry, error) {
	am.mu.RLock()
	snap := am.activeSnap
	am.mu.RUnlock()

	if snap == nil {
		return nil, fmt.Errorf("no active snapshot")
	}

	entry, ok := snap.Get(key)
	if !ok {
		return nil, fmt.Errorf("secret not found")
	}

	if entry.LastUsedAt == nil || time.Since(*entry.LastUsedAt) > time.Minute {
		now := time.Now().UTC()
		entry.LastUsedAt = &now

		am.mu.Lock()
		if rs := am.activeSnap; rs != nil {
			if e, ok := rs.secrets[key]; ok {
				e.LastUsedAt = entry.LastUsedAt
			}
		}
		am.mu.Unlock()
	}

	am.store.AddAuditEntry(&AuditEntry{
		Operation: OpAccess,
		SecretKey: key,
		Actor:     actor,
		Success:   true,
	})

	return entry, nil
}

func (am *ActivationManager) Status() map[string]interface{} {
	am.mu.RLock()
	defer am.mu.RUnlock()

	status := map[string]interface{}{
		"has_active_snapshot": am.activeSnap != nil,
		"snapshot_version":    uint64(0),
		"snapshot_checksum":   "",
		"is_locked":           false,
		"lock_state":          "",
		"approval_count":      0,
		"required_approvals":  am.config.ApprovalCount,
	}

	if am.activeSnap != nil {
		status["snapshot_version"] = am.activeSnap.Version()
		status["snapshot_checksum"] = am.activeSnap.Checksum()
	}

	if am.activeLock != nil {
		status["is_locked"] = am.activeLock.State == LockPending || am.activeLock.State == LockActivated
		status["lock_state"] = string(am.activeLock.State)
		status["approval_count"] = len(am.activeLock.Approvals)
		status["lock_id"] = am.activeLock.ID
		if am.activeLock.ExpiresAt != nil {
			status["lock_expires_at"] = am.activeLock.ExpiresAt
		}
	}

	return status
}

func computeRuntimeChecksum(secrets map[string]*SecretEntry) string {
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	// Simple sort without importing sort again
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	h := sha256.New()
	for _, k := range keys {
		entry := secrets[k]
		fmt.Fprintf(h, "%s:%s:%d\n", entry.Key, entry.UpdatedAt.Format(time.RFC3339Nano), len(entry.Value))
	}
	return hex.EncodeToString(h.Sum(nil))
}
