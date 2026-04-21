package secrets

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
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

var secretRefRegex = regexp.MustCompile(`\$\{SECRET:([^}]+)\}`)

func (rs *RuntimeSnapshot) ScanReferences() map[string][]string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	refs := make(map[string][]string)
	for _, entry := range rs.secrets {
		if entry.Value == "" {
			continue
		}
		matches := secretRefRegex.FindAllStringSubmatch(entry.Value, -1)
		if len(matches) > 0 {
			refKeys := make([]string, 0, len(matches))
			for _, m := range matches {
				refKeys = append(refKeys, m[1])
			}
			refs[entry.Key] = refKeys
		}
	}
	return refs
}

func (rs *RuntimeSnapshot) ValidateReferences(mode ValidationMode, requiredKeys []string) *ValidationResult {
	result := &ValidationResult{
		Valid: true,
		Refs:  make(map[string][]string),
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	keySet := make(map[string]bool)
	for k := range rs.secrets {
		keySet[k] = true
	}

	for _, entry := range rs.secrets {
		if entry.Value == "" {
			continue
		}
		matches := secretRefRegex.FindAllStringSubmatch(entry.Value, -1)
		if len(matches) == 0 {
			continue
		}

		refKeys := make([]string, 0, len(matches))
		for _, m := range matches {
			refKey := m[1]
			refKeys = append(refKeys, refKey)

			if !keySet[refKey] {
				if mode == ValidationStrict {
					result.AddError(&ValidationError{
						SecretKey: entry.Key,
						RefKey:    refKey,
						Message:   "referenced secret does not exist",
					})
				}
			}
		}
		result.Refs[entry.Key] = refKeys
		result.Scanned++
	}

	for _, reqKey := range requiredKeys {
		if !keySet[reqKey] {
			result.AddError(&ValidationError{
				SecretKey: reqKey,
				Message:   "required secret is missing",
			})
		}
	}

	if mode == ValidationStrict {
		for _, entry := range rs.secrets {
			if entry.Value == "" {
				result.AddError(&ValidationError{
					SecretKey: entry.Key,
					Message:   "secret value is empty",
				})
			}
		}
	}

	return result
}

func (rs *RuntimeSnapshot) ResolveValueStrict(template string) (string, error) {
	if !strings.Contains(template, "${SECRET:") {
		return template, nil
	}

	result := template
	for strings.Contains(result, "${SECRET:") {
		start := strings.Index(result, "${SECRET:")
		end := strings.Index(result[start:], "}")
		if end == -1 {
			return "", fmt.Errorf("malformed secret reference in: %s", template)
		}
		end += start

		key := result[start+9 : end]
		if entry, ok := rs.Get(key); ok {
			result = result[:start] + entry.Value + result[end+1:]
		} else {
			return "", fmt.Errorf("unresolved secret reference: %q", key)
		}
	}
	return result, nil
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
	mu           sync.RWMutex
	store        *Store
	activeLock   *ActivationLock
	activeSnap   *RuntimeSnapshot
	fallbackSnap *RuntimeSnapshot
	approvals    map[string]bool
	config       *StoreConfig
	fallbackCfg  *FallbackConfig
	recovery     *RecoveryStatus
}

func NewActivationManager(store *Store, initialSnap *RuntimeSnapshot) *ActivationManager {
	return &ActivationManager{
		store:      store,
		activeSnap: initialSnap,
		approvals:  make(map[string]bool),
		config:     store.Config(),
		recovery: &RecoveryStatus{
			State: RecoveryNormal,
		},
	}
}

func NewActivationManagerWithFallback(store *Store, initialSnap *RuntimeSnapshot, fbCfg *FallbackConfig) *ActivationManager {
	if fbCfg == nil {
		fbCfg = DefaultFallbackConfig()
	}
	am := &ActivationManager{
		store:       store,
		activeSnap:  initialSnap,
		approvals:   make(map[string]bool),
		config:      store.Config(),
		fallbackCfg: fbCfg,
		recovery: &RecoveryStatus{
			State: RecoveryNormal,
		},
	}
	if initialSnap != nil {
		am.fallbackSnap = NewRuntimeSnapshot(initialSnap.GetAll(), "fallback_copy")
		am.recovery.CurrentSnapID = initialSnap.ToSnapshot().ID
	}
	return am
}

func (am *ActivationManager) ValidateStartup(cfg *StartupConfig) error {
	if cfg == nil {
		cfg = DefaultStartupConfig()
	}
	if cfg.ValidationMode == ValidationOff {
		return nil
	}

	if am.activeSnap == nil {
		if cfg.FailFast {
			return fmt.Errorf("secrets startup validation failed: no active snapshot loaded")
		}
		return nil
	}

	result := am.activeSnap.ValidateReferences(cfg.ValidationMode, cfg.RequiredKeys)
	if !result.Valid {
		var msgs []string
		for _, err := range result.Errors {
			msgs = append(msgs, err.Error())
		}

		if cfg.FailFast {
			return fmt.Errorf("secrets startup validation failed (%d errors): %s",
				len(result.Errors), strings.Join(msgs, "; "))
		}

		am.store.AddAuditEntry(&AuditEntry{
			Operation: OpCreate,
			Actor:     "system",
			Details: map[string]interface{}{
				"validation_errors": msgs,
				"mode":              string(cfg.ValidationMode),
			},
			Success: false,
			Error:   strings.Join(msgs, "; "),
		})
	}

	return nil
}

func (am *ActivationManager) GetActiveSnapshot() *RuntimeSnapshot {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.activeSnap
}

func (am *ActivationManager) CreateSnapshot(source string) (*Snapshot, error) {
	am.mu.RLock()
	snap := am.activeSnap
	am.mu.RUnlock()
	if snap == nil {
		return nil, fmt.Errorf("no active snapshot")
	}
	return am.store.CreateSnapshot(source)
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
	if am.activeSnap == nil {
		return nil, fmt.Errorf("no active snapshot")
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
		"recovery_state":      am.recovery.State,
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

	if am.fallbackSnap != nil {
		status["has_fallback"] = true
		status["fallback_checksum"] = am.fallbackSnap.Checksum()
	}

	return status
}

func (am *ActivationManager) RecoveryStatus() *RecoveryStatus {
	am.mu.RLock()
	defer am.mu.RUnlock()
	status := *am.recovery
	if am.recovery.LastRecovery != nil {
		t := *am.recovery.LastRecovery
		status.LastRecovery = &t
	}
	if am.recovery.LastEvent != nil {
		ev := *am.recovery.LastEvent
		status.LastEvent = &ev
	}
	return &status
}

func (am *ActivationManager) DetectAndFallback(trigger string) (*RuntimeSnapshot, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.fallbackCfg == nil || !am.fallbackCfg.Enabled {
		return nil, fmt.Errorf("fallback not enabled")
	}

	if am.recovery.State == RecoveryFalling {
		return nil, fmt.Errorf("fallback already in progress")
	}

	if am.recovery.AttemptCount >= am.fallbackCfg.MaxAttempts {
		return nil, fmt.Errorf("max fallback attempts (%d) exceeded", am.fallbackCfg.MaxAttempts)
	}

	am.recovery.State = RecoveryDetecting
	am.recovery.AttemptCount++

	event := &FallbackEvent{
		ID:        generateID("fb"),
		Trigger:   trigger,
		Strategy:  am.fallbackCfg.Strategy,
		Timestamp: time.Now().UTC(),
		Details:   make(map[string]string),
	}

	var recovered *RuntimeSnapshot
	var err error

	switch am.fallbackCfg.Strategy {
	case FallbackLastSnapshot:
		recovered, err = am.fallbackFromLastSnapshot(event)
	case FallbackEnvVars:
		recovered, err = am.fallbackFromEnvVars(event)
	case FallbackDefaults:
		recovered, err = am.fallbackFromDefaults(event)
	case FallbackEmpty:
		recovered, err = am.fallbackToEmpty(event)
	default:
		err = fmt.Errorf("unknown fallback strategy: %s", am.fallbackCfg.Strategy)
	}

	if err != nil {
		event.Success = false
		event.Error = err.Error()
		am.recovery.State = RecoveryFailed
		am.recovery.LastEvent = event

		am.store.AddAuditEntry(&AuditEntry{
			Operation: OpActivate,
			Actor:     "system:fallback",
			Details: map[string]interface{}{
				"trigger":  trigger,
				"strategy": string(am.fallbackCfg.Strategy),
				"error":    err.Error(),
			},
			Success: false,
			Error:   err.Error(),
		})

		return nil, fmt.Errorf("fallback failed: %w", err)
	}

	am.recovery.State = RecoveryRecovered
	now := time.Now().UTC()
	am.recovery.LastRecovery = &now
	am.recovery.LastEvent = event
	am.activeSnap = recovered
	am.recovery.FallbackSnapID = event.ID

	event.Success = true
	event.RestoredKey = len(recovered.GetAll())

	am.store.AddAuditEntry(&AuditEntry{
		Operation: OpActivate,
		Actor:     "system:fallback",
		Details: map[string]interface{}{
			"trigger":       trigger,
			"strategy":      string(am.fallbackCfg.Strategy),
			"restored_keys": event.RestoredKey,
		},
		Success: true,
	})

	return recovered, nil
}

func (am *ActivationManager) fallbackFromLastSnapshot(event *FallbackEvent) (*RuntimeSnapshot, error) {
	if am.fallbackSnap != nil {
		event.Details["source"] = "cached_fallback"
		event.Details["checksum"] = am.fallbackSnap.Checksum()
		return NewRuntimeSnapshot(am.fallbackSnap.GetAll(), "fallback_restore"), nil
	}

	snaps := am.store.ListSnapshots()
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no snapshots available for fallback")
	}

	latest := snaps[0]
	if len(latest.Secrets) == 0 {
		return nil, fmt.Errorf("latest snapshot has no secrets")
	}

	event.Details["source"] = "store_snapshot"
	event.Details["snapshot_id"] = latest.ID
	event.Details["version"] = fmt.Sprintf("%d", latest.Version)

	secrets := make(map[string]*SecretEntry)
	for k, v := range latest.Secrets {
		secrets[k] = v
	}

	return NewRuntimeSnapshot(secrets, "fallback_restore"), nil
}

func (am *ActivationManager) fallbackFromEnvVars(event *FallbackEvent) (*RuntimeSnapshot, error) {
	prefix := am.fallbackCfg.EnvPrefix
	if prefix == "" {
		prefix = "ANYCLAW_SECRET_"
	}

	secrets := make(map[string]*SecretEntry)
	var restored int

	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, prefix) {
			continue
		}

		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimPrefix(parts[0], prefix)
		value := parts[1]

		if value == "" {
			continue
		}

		secrets[key] = &SecretEntry{
			Key:       key,
			Value:     value,
			Scope:     ScopeGlobal,
			Source:    SourceEnv,
			UpdatedAt: time.Now().UTC(),
			CreatedAt: time.Now().UTC(),
		}
		restored++
	}

	if restored == 0 {
		return nil, fmt.Errorf("no secrets found in environment with prefix %q", prefix)
	}

	event.Details["source"] = "environment"
	event.Details["prefix"] = prefix
	event.Details["restored"] = fmt.Sprintf("%d", restored)

	return NewRuntimeSnapshot(secrets, "fallback_env"), nil
}

func (am *ActivationManager) fallbackFromDefaults(event *FallbackEvent) (*RuntimeSnapshot, error) {
	defaultVal := am.fallbackCfg.DefaultValue
	if defaultVal == "" {
		return nil, fmt.Errorf("no default value configured")
	}

	secrets := make(map[string]*SecretEntry)
	if am.fallbackSnap != nil {
		for k := range am.fallbackSnap.GetAll() {
			secrets[k] = &SecretEntry{
				Key:       k,
				Value:     defaultVal,
				Scope:     ScopeGlobal,
				Source:    SourceManual,
				UpdatedAt: time.Now().UTC(),
				CreatedAt: time.Now().UTC(),
				Metadata:  map[string]string{"fallback": "default_value"},
			}
		}
	}

	if len(secrets) == 0 {
		return nil, fmt.Errorf("no known keys to apply defaults to")
	}

	event.Details["source"] = "default_value"
	event.Details["default_length"] = fmt.Sprintf("%d", len(defaultVal))

	return NewRuntimeSnapshot(secrets, "fallback_defaults"), nil
}

func (am *ActivationManager) fallbackToEmpty(event *FallbackEvent) (*RuntimeSnapshot, error) {
	event.Details["source"] = "empty"
	return NewRuntimeSnapshot(map[string]*SecretEntry{}, "fallback_empty"), nil
}

func (am *ActivationManager) RestoreFromFallback() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.fallbackSnap == nil {
		return fmt.Errorf("no fallback snapshot available")
	}

	am.activeSnap = NewRuntimeSnapshot(am.fallbackSnap.GetAll(), "manual_restore")
	am.recovery.State = RecoveryRecovered
	now := time.Now().UTC()
	am.recovery.LastRecovery = &now

	return am.store.AddAuditEntry(&AuditEntry{
		Operation: OpSnapshot,
		Actor:     "system:restore",
		Details:   map[string]interface{}{"source": "fallback"},
		Success:   true,
	})
}

func (am *ActivationManager) ResetRecovery() {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.recovery.State = RecoveryNormal
	am.recovery.AttemptCount = 0
	am.recovery.LastEvent = nil
	am.recovery.FallbackSnapID = ""

	if am.activeSnap != nil {
		am.fallbackSnap = NewRuntimeSnapshot(am.activeSnap.GetAll(), "reset_fallback")
		am.recovery.CurrentSnapID = am.activeSnap.ToSnapshot().ID
	}
}

func (am *ActivationManager) syncActiveSecretToStore(entry *SecretEntry) error {
	if am.activeSnap == nil {
		return fmt.Errorf("no active snapshot")
	}
	if entry == nil {
		return fmt.Errorf("secret entry is required")
	}
	if err := am.store.SetSecret(entry); err != nil {
		return err
	}

	am.activeSnap.Update(am.activeSnap.GetAll())
	return nil
}

func (am *ActivationManager) RotateSecret(req *RotationRequest) (*RotationResult, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if req == nil || req.Key == "" || req.NewValue == "" {
		return nil, fmt.Errorf("key and new_value are required")
	}
	if am.activeSnap == nil {
		return nil, fmt.Errorf("no active snapshot")
	}

	entry, ok := am.activeSnap.Get(req.Key)
	if !ok {
		return nil, fmt.Errorf("secret %q not found in active snapshot", req.Key)
	}

	oldVersion := uint64(0)
	if vh, exists := am.store.GetVersionHistory(req.Key); exists {
		oldVersion = vh.Current
	}
	newVersion := oldVersion + 1

	if err := am.store.RecordRotation(req.Key, oldVersion, newVersion, req.NewValue, req.RequestedBy, req.Metadata); err != nil {
		return nil, fmt.Errorf("record rotation: %w", err)
	}

	entry.Value = req.NewValue
	entry.UpdatedAt = time.Now().UTC()
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]string)
	}
	entry.Metadata["last_rotated_by"] = req.RequestedBy
	entry.Metadata["last_rotation_reason"] = req.Reason
	entry.Metadata["version"] = fmt.Sprintf("%d", newVersion)

	if err := am.syncActiveSecretToStore(entry); err != nil {
		return nil, fmt.Errorf("persist rotated secret: %w", err)
	}

	result := &RotationResult{
		Key:        req.Key,
		OldVersion: oldVersion,
		NewVersion: newVersion,
		Activated:  req.ActivateNow,
		RotatedAt:  time.Now().UTC(),
	}

	if policy, ok := am.store.GetRotationPolicy(req.Key); ok && policy.GracePeriod > 0 {
		graceEnd := time.Now().Add(policy.GracePeriod)
		result.GraceEnd = &graceEnd
	}

	am.store.AddAuditEntry(&AuditEntry{
		Operation: OpRotate,
		SecretKey: req.Key,
		Actor:     req.RequestedBy,
		Details: map[string]interface{}{
			"old_version": oldVersion,
			"new_version": newVersion,
			"reason":      req.Reason,
		},
		Success: true,
	})

	return result, nil
}

func (am *ActivationManager) GetVersionHistory(key string) (*VersionHistory, error) {
	vh, ok := am.store.GetVersionHistory(key)
	if !ok {
		return nil, fmt.Errorf("version history not found for key %q", key)
	}

	if policy, ok := am.store.GetRotationPolicy(key); ok {
		vh.Policy = policy
	}

	return vh, nil
}

func (am *ActivationManager) ListVersionHistories() []*VersionHistory {
	return am.store.ListVersionHistories()
}

func (am *ActivationManager) RollbackVersion(key string, targetVersion uint64, actor string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.activeSnap == nil {
		return fmt.Errorf("no active snapshot")
	}

	if err := am.store.RollbackVersion(key, targetVersion, actor); err != nil {
		return err
	}

	activeVer, ok := am.store.GetActiveVersion(key)
	if !ok {
		return fmt.Errorf("active version not found after rollback")
	}

	entry, ok := am.activeSnap.Get(key)
	if !ok {
		return fmt.Errorf("secret %q not found in active snapshot", key)
	}

	entry.Value = activeVer.Value
	entry.UpdatedAt = time.Now().UTC()
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]string)
	}
	entry.Metadata["rolled_back_from"] = fmt.Sprintf("v%d", targetVersion)
	entry.Metadata["rolled_back_by"] = actor

	if err := am.syncActiveSecretToStore(entry); err != nil {
		return fmt.Errorf("persist rolled back secret: %w", err)
	}

	am.store.AddAuditEntry(&AuditEntry{
		Operation: OpRotate,
		SecretKey: key,
		Actor:     actor,
		Details: map[string]interface{}{
			"action":         "rollback",
			"target_version": targetVersion,
		},
		Success: true,
	})

	return nil
}

func (am *ActivationManager) SetRotationPolicy(policy *RotationPolicy) error {
	return am.store.SetRotationPolicy(policy)
}

func (am *ActivationManager) GetRotationPolicy(key string) (*RotationPolicy, bool) {
	return am.store.GetRotationPolicy(key)
}

func (am *ActivationManager) ListRotationPolicies() []*RotationPolicy {
	return am.store.ListRotationPolicies()
}

func (am *ActivationManager) DeleteRotationPolicy(key string) error {
	return am.store.DeleteRotationPolicy(key)
}

func (am *ActivationManager) CheckScheduledRotations(actor string) ([]*RotationResult, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.activeSnap == nil {
		return nil, fmt.Errorf("no active snapshot")
	}

	policies := am.store.ListRotationPolicies()
	var results []*RotationResult
	now := time.Now().UTC()

	for _, policy := range policies {
		if policy.Strategy != RotationScheduled {
			continue
		}
		if policy.NextRotation == nil || now.Before(*policy.NextRotation) {
			continue
		}

		entry, ok := am.activeSnap.Get(policy.Key)
		if !ok {
			continue
		}

		newValue := generateRotatedValue(entry.Value)
		if newValue == "" {
			continue
		}

		oldVersion := uint64(0)
		if vh, exists := am.store.GetVersionHistory(policy.Key); exists {
			oldVersion = vh.Current
		}
		newVersion := oldVersion + 1

		err := am.store.RecordRotation(policy.Key, oldVersion, newVersion, newValue, actor,
			map[string]string{"auto": "true", "strategy": "scheduled"})
		if err != nil {
			continue
		}

		if policy.AutoActivate {
			entry.Value = newValue
			entry.UpdatedAt = now
			if entry.Metadata == nil {
				entry.Metadata = make(map[string]string)
			}
			entry.Metadata["last_rotated_by"] = actor
			entry.Metadata["last_rotation_reason"] = "scheduled rotation"
			entry.Metadata["rotation_strategy"] = "scheduled"
			entry.Metadata["version"] = fmt.Sprintf("%d", newVersion)

			if err := am.syncActiveSecretToStore(entry); err != nil {
				continue
			}
		}

		results = append(results, &RotationResult{
			Key:        policy.Key,
			OldVersion: oldVersion,
			NewVersion: newVersion,
			Activated:  policy.AutoActivate,
			RotatedAt:  now,
		})
	}

	if len(results) > 0 {
		am.store.AddAuditEntry(&AuditEntry{
			Operation: OpRotate,
			Actor:     actor,
			Details: map[string]interface{}{
				"action":       "scheduled_check",
				"rotated_keys": len(results),
			},
			Success: true,
		})
	}

	return results, nil
}

func (am *ActivationManager) CleanupOldVersions(key string, keep int) error {
	return am.store.DeleteOldVersions(key, keep)
}

func generateRotatedValue(current string) string {
	if current == "" {
		return ""
	}
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
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
