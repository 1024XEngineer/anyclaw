package secrets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type persistedData struct {
	Secrets          []*SecretEntry    `json:"secrets"`
	Snapshots        []*Snapshot       `json:"snapshots"`
	Locks            []*ActivationLock `json:"locks"`
	VersionHistories []*VersionHistory `json:"version_histories,omitempty"`
	RotationPolicies []*RotationPolicy `json:"rotation_policies,omitempty"`
	AuditLog         []*AuditEntry     `json:"audit_log,omitempty"`
	LastUpdate       time.Time         `json:"last_update"`
}

type Store struct {
	mu            sync.RWMutex
	path          string
	encryptionKey []byte
	config        *StoreConfig
	data          *persistedData
}

func NewStore(cfg *StoreConfig) (*Store, error) {
	if cfg == nil {
		cfg = DefaultStoreConfig()
	}

	path, err := resolveStorePath(cfg.Path)
	if err != nil {
		return nil, err
	}

	var encKey []byte
	if cfg.EncryptionKey != "" {
		keyBytes, err := decodeKey(cfg.EncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("invalid encryption key: %w", err)
		}
		encKey = keyBytes
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	store := &Store{
		path:          path,
		encryptionKey: encKey,
		config:        cfg,
		data: &persistedData{
			Secrets:   []*SecretEntry{},
			Snapshots: []*Snapshot{},
			Locks:     []*ActivationLock{},
			AuditLog:  []*AuditEntry{},
		},
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state persistedData
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	s.data = &state

	if s.encryptionKey != nil {
		for _, secret := range s.data.Secrets {
			if secret.Value != "" && isEncryptedValue(secret.Value) {
				decrypted, err := DecryptValue(secret.Value, s.encryptionKey)
				if err != nil {
					continue
				}
				secret.Value = decrypted
			}
		}
		for _, vh := range s.data.VersionHistories {
			for _, v := range vh.Versions {
				if v.Value != "" && isEncryptedValue(v.Value) {
					decrypted, err := DecryptValue(v.Value, s.encryptionKey)
					if err != nil {
						continue
					}
					v.Value = decrypted
				}
			}
		}
	}

	return nil
}

func (s *Store) saveLocked() error {
	saveData := &persistedData{
		Secrets:          cloneEntries(s.data.Secrets),
		Snapshots:        cloneSnapshots(s.data.Snapshots),
		Locks:            cloneLocks(s.data.Locks),
		VersionHistories: cloneVersionHistories(s.data.VersionHistories),
		RotationPolicies: s.data.RotationPolicies,
		AuditLog:         cloneAuditEntries(s.data.AuditLog),
		LastUpdate:       time.Now().UTC(),
	}

	if s.encryptionKey != nil {
		for _, secret := range saveData.Secrets {
			if secret.Value != "" && !isEncryptedValue(secret.Value) {
				encrypted, err := EncryptValue(secret.Value, s.encryptionKey)
				if err != nil {
					return fmt.Errorf("encrypt secret %s: %w", secret.Key, err)
				}
				secret.Value = encrypted
			}
		}
		for _, vh := range saveData.VersionHistories {
			for _, v := range vh.Versions {
				if v.Value != "" && !isEncryptedValue(v.Value) {
					encrypted, err := EncryptValue(v.Value, s.encryptionKey)
					if err != nil {
						return fmt.Errorf("encrypt version %s v%d: %w", vh.Key, v.Version, err)
					}
					v.Value = encrypted
				}
			}
		}
	}

	jsonData, err := json.MarshalIndent(saveData, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0o600); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

func (s *Store) GetSecret(key string, scope SecretScope, scopeRef string) (*SecretEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.data.Secrets {
		if entry.Key == key && entry.Scope == scope && entry.ScopeRef == scopeRef {
			if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
				return nil, false
			}
			return cloneEntry(entry), true
		}
	}
	return nil, false
}

func (s *Store) SetSecret(entry *SecretEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if entry.ID == "" {
		entry.ID = generateID("sec")
	}
	entry.UpdatedAt = now
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}

	for i, existing := range s.data.Secrets {
		if existing.Key == entry.Key && existing.Scope == entry.Scope && existing.ScopeRef == entry.ScopeRef {
			entry.ID = existing.ID
			entry.CreatedAt = existing.CreatedAt
			s.data.Secrets[i] = cloneEntry(entry)
			return s.saveLocked()
		}
	}

	s.data.Secrets = append(s.data.Secrets, cloneEntry(entry))
	return s.saveLocked()
}

func (s *Store) DeleteSecret(key string, scope SecretScope, scopeRef string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]*SecretEntry, 0, len(s.data.Secrets))
	found := false
	for _, entry := range s.data.Secrets {
		if entry.Key == key && entry.Scope == scope && entry.ScopeRef == scopeRef {
			found = true
			continue
		}
		filtered = append(filtered, cloneEntry(entry))
	}

	if !found {
		return fmt.Errorf("secret not found")
	}

	s.data.Secrets = filtered
	return s.saveLocked()
}

func (s *Store) ListSecrets(scope SecretScope, scopeRef string) []*SecretEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*SecretEntry
	for _, entry := range s.data.Secrets {
		if scope != "" && entry.Scope != scope {
			continue
		}
		if scopeRef != "" && entry.ScopeRef != scopeRef {
			continue
		}
		if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
			continue
		}
		result = append(result, cloneEntry(entry))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result
}

func (s *Store) CreateSnapshot(source string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	secrets := make(map[string]*SecretEntry)
	for _, entry := range s.data.Secrets {
		secrets[entry.Key] = cloneEntry(entry)
	}

	version := uint64(0)
	if len(s.data.Snapshots) > 0 {
		version = s.data.Snapshots[len(s.data.Snapshots)-1].Version + 1
	}

	snapshot := &Snapshot{
		ID:        generateID("snap"),
		Version:   version,
		Secrets:   secrets,
		CreatedAt: time.Now().UTC(),
		Source:    source,
	}

	snapshot.Checksum = computeSnapshotChecksum(snapshot)

	s.data.Snapshots = append(s.data.Snapshots, snapshot)

	if s.config.MaxSnapshots > 0 && len(s.data.Snapshots) > s.config.MaxSnapshots {
		s.data.Snapshots = s.data.Snapshots[len(s.data.Snapshots)-s.config.MaxSnapshots:]
	}

	if err := s.saveLocked(); err != nil {
		return nil, err
	}

	return cloneSnapshot(snapshot), nil
}

func (s *Store) GetSnapshot(id string) (*Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, snap := range s.data.Snapshots {
		if snap.ID == id {
			return cloneSnapshot(snap), true
		}
	}
	return nil, false
}

func (s *Store) GetSnapshotByVersion(version uint64) (*Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, snap := range s.data.Snapshots {
		if snap.Version == version {
			return cloneSnapshot(snap), true
		}
	}
	return nil, false
}

func (s *Store) ListSnapshots() []*Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Snapshot, 0, len(s.data.Snapshots))
	for _, snap := range s.data.Snapshots {
		result = append(result, cloneSnapshot(snap))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})
	return result
}

func (s *Store) RestoreSnapshot(snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var target *Snapshot
	for _, snap := range s.data.Snapshots {
		if snap.ID == snapshotID {
			target = snap
			break
		}
	}

	if target == nil {
		return fmt.Errorf("snapshot not found")
	}

	secrets := make([]*SecretEntry, 0, len(target.Secrets))
	for _, entry := range target.Secrets {
		secrets = append(secrets, cloneEntry(entry))
	}

	s.data.Secrets = secrets
	return s.saveLocked()
}

func (s *Store) CreateLock(lock *ActivationLock) error {
	if lock == nil {
		return fmt.Errorf("lock is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if lock.ID == "" {
		lock.ID = generateID("lock")
	}
	lock.RequestedAt = now
	if lock.State == "" {
		lock.State = LockPending
	}
	if lock.ExpiresAt == nil && s.config.LockTimeout > 0 {
		expires := now.Add(s.config.LockTimeout)
		lock.ExpiresAt = &expires
	}

	s.data.Locks = append(s.data.Locks, cloneLock(lock))
	return s.saveLocked()
}

func (s *Store) GetLock(id string) (*ActivationLock, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, lock := range s.data.Locks {
		if lock.ID == id {
			return cloneLock(lock), true
		}
	}
	return nil, false
}

func (s *Store) UpdateLock(id string, updater func(*ActivationLock) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, lock := range s.data.Locks {
		if lock.ID == id {
			if err := updater(lock); err != nil {
				return err
			}
			s.data.Locks[i] = cloneLock(lock)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("lock not found")
}

func (s *Store) ListLocks(state ActivationLockState) []*ActivationLock {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ActivationLock
	for _, lock := range s.data.Locks {
		if state != "" && lock.State != state {
			continue
		}
		result = append(result, cloneLock(lock))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RequestedAt.After(result[j].RequestedAt)
	})
	return result
}

func (s *Store) GetActiveLock(snapshotID string) (*ActivationLock, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, lock := range s.data.Locks {
		if lock.SnapshotID == snapshotID &&
			(lock.State == LockPending || lock.State == LockActivated) {
			if lock.ExpiresAt != nil && time.Now().After(*lock.ExpiresAt) {
				continue
			}
			return cloneLock(lock), true
		}
	}
	return nil, false
}

func (s *Store) AddAuditEntry(entry *AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = generateID("audit")
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	s.data.AuditLog = append(s.data.AuditLog, cloneAuditEntry(entry))

	const maxAuditEntries = 10000
	if len(s.data.AuditLog) > maxAuditEntries {
		s.data.AuditLog = s.data.AuditLog[len(s.data.AuditLog)-maxAuditEntries:]
	}

	return s.saveLocked()
}

func (s *Store) ListAuditEntries(limit int) []*AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n := limit
	if n <= 0 || n > len(s.data.AuditLog) {
		n = len(s.data.AuditLog)
	}

	result := make([]*AuditEntry, n)
	start := len(s.data.AuditLog) - n
	for i := 0; i < n; i++ {
		result[i] = cloneAuditEntry(s.data.AuditLog[start+i])
	}

	return result
}

func (s *Store) Config() *StoreConfig {
	return s.config
}

func (s *Store) SetRotationPolicy(policy *RotationPolicy) error {
	if policy == nil || policy.Key == "" {
		return fmt.Errorf("policy key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.data.RotationPolicies {
		if existing.Key == policy.Key {
			s.data.RotationPolicies[i] = policy
			return s.saveLocked()
		}
	}

	s.data.RotationPolicies = append(s.data.RotationPolicies, policy)
	return s.saveLocked()
}

func (s *Store) GetRotationPolicy(key string) (*RotationPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, policy := range s.data.RotationPolicies {
		if policy.Key == key {
			return policy, true
		}
	}
	return nil, false
}

func (s *Store) DeleteRotationPolicy(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]*RotationPolicy, 0, len(s.data.RotationPolicies))
	found := false
	for _, policy := range s.data.RotationPolicies {
		if policy.Key == key {
			found = true
			continue
		}
		filtered = append(filtered, policy)
	}

	if !found {
		return fmt.Errorf("rotation policy not found")
	}

	s.data.RotationPolicies = filtered
	return s.saveLocked()
}

func (s *Store) ListRotationPolicies() []*RotationPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RotationPolicy, len(s.data.RotationPolicies))
	copy(result, s.data.RotationPolicies)
	return result
}

func (s *Store) GetVersionHistory(key string) (*VersionHistory, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, vh := range s.data.VersionHistories {
		if vh.Key == key {
			return cloneVersionHistory(vh), true
		}
	}
	return nil, false
}

func (s *Store) ListVersionHistories() []*VersionHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*VersionHistory, 0, len(s.data.VersionHistories))
	for _, vh := range s.data.VersionHistories {
		result = append(result, cloneVersionHistory(vh))
	}
	return result
}

func (s *Store) RecordRotation(key string, oldVersion uint64, newVersion uint64, newValue string, actor string, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()

	var vh *VersionHistory
	for i, existing := range s.data.VersionHistories {
		if existing.Key == key {
			vh = existing
			s.data.VersionHistories[i] = vh
			break
		}
	}

	if vh == nil {
		vh = &VersionHistory{
			Key:      key,
			Versions: []*SecretVersion{},
		}
		s.data.VersionHistories = append(s.data.VersionHistories, vh)
	}

	for i, v := range vh.Versions {
		if v.Active {
			v.Active = false
			t := now
			v.RotatedAt = &t
			v.RotatedBy = actor
			vh.Versions[i] = v
		}
	}

	newVer := &SecretVersion{
		Version:   newVersion,
		Value:     newValue,
		CreatedBy: actor,
		CreatedAt: now,
		Active:    true,
		Metadata:  metadata,
	}
	vh.Versions = append(vh.Versions, newVer)

	vh.Current = newVersion
	vh.TotalRotates++
	vh.LastRotation = &now

	var policy *RotationPolicy
	for _, p := range s.data.RotationPolicies {
		if p.Key == key {
			policy = p
			break
		}
	}

	if policy != nil && policy.MaxVersions > 0 && len(vh.Versions) > policy.MaxVersions {
		vh.Versions = vh.Versions[len(vh.Versions)-policy.MaxVersions:]
	}

	if policy != nil && policy.Interval > 0 {
		next := now.Add(policy.Interval)
		vh.NextRotation = &next
		policy.NextRotation = &next
	}

	return s.saveLocked()
}

func (s *Store) GetActiveVersion(key string) (*SecretVersion, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, vh := range s.data.VersionHistories {
		if vh.Key == key {
			for _, v := range vh.Versions {
				if v.Active {
					return v, true
				}
			}
		}
	}
	return nil, false
}

func (s *Store) RollbackVersion(key string, targetVersion uint64, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, vh := range s.data.VersionHistories {
		if vh.Key == key {
			var target *SecretVersion
			for _, v := range vh.Versions {
				if v.Version == targetVersion {
					target = v
					break
				}
			}

			if target == nil {
				return fmt.Errorf("version %d not found for key %s", targetVersion, key)
			}

			now := time.Now().UTC()
			for i, v := range vh.Versions {
				if v.Active {
					v.Active = false
					t := now
					v.RotatedAt = &t
					v.RotatedBy = actor
					vh.Versions[i] = v
				}
			}

			target.Active = true
			vh.Current = targetVersion
			vh.LastRotation = &now

			return s.saveLocked()
		}
	}

	return fmt.Errorf("version history not found for key %s", key)
}

func (s *Store) DeleteOldVersions(key string, keep int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, vh := range s.data.VersionHistories {
		if vh.Key == key {
			if keep <= 0 || len(vh.Versions) <= keep {
				return nil
			}

			sort.Slice(vh.Versions, func(i, j int) bool {
				return vh.Versions[i].Version > vh.Versions[j].Version
			})

			vh.Versions = vh.Versions[:keep]
			return s.saveLocked()
		}
	}

	return fmt.Errorf("version history not found for key %s", key)
}

func computeSnapshotChecksum(snap *Snapshot) string {
	keys := make([]string, 0, len(snap.Secrets))
	for k := range snap.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		entry := snap.Secrets[k]
		fmt.Fprintf(h, "%s:%s:%d\n", entry.Key, entry.UpdatedAt.Format(time.RFC3339Nano), len(entry.Value))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func resolveStorePath(configPath string) (string, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(absConfig), ".anyclaw", "secrets", "store.json"), nil
}

func decodeKey(key string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(key)
}

func isEncryptedValue(value string) bool {
	if len(value) < 44 {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return len(decoded) > 12
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func cloneEntry(e *SecretEntry) *SecretEntry {
	if e == nil {
		return nil
	}
	c := *e
	if e.Tags != nil {
		c.Tags = make([]string, len(e.Tags))
		copy(c.Tags, e.Tags)
	}
	if e.Metadata != nil {
		c.Metadata = make(map[string]string)
		for k, v := range e.Metadata {
			c.Metadata[k] = v
		}
	}
	if e.ExpiresAt != nil {
		t := *e.ExpiresAt
		c.ExpiresAt = &t
	}
	if e.LastUsedAt != nil {
		t := *e.LastUsedAt
		c.LastUsedAt = &t
	}
	return &c
}

func cloneEntries(entries []*SecretEntry) []*SecretEntry {
	result := make([]*SecretEntry, len(entries))
	for i, e := range entries {
		result[i] = cloneEntry(e)
	}
	return result
}

func cloneSnapshot(s *Snapshot) *Snapshot {
	if s == nil {
		return nil
	}
	c := *s
	if s.Secrets != nil {
		c.Secrets = make(map[string]*SecretEntry)
		for k, v := range s.Secrets {
			c.Secrets[k] = cloneEntry(v)
		}
	}
	return &c
}

func cloneSnapshots(snaps []*Snapshot) []*Snapshot {
	result := make([]*Snapshot, len(snaps))
	for i, s := range snaps {
		result[i] = cloneSnapshot(s)
	}
	return result
}

func cloneLock(l *ActivationLock) *ActivationLock {
	if l == nil {
		return nil
	}
	c := *l
	if l.Approvals != nil {
		c.Approvals = make([]LockApproval, len(l.Approvals))
		copy(c.Approvals, l.Approvals)
	}
	if l.ActivatedAt != nil {
		t := *l.ActivatedAt
		c.ActivatedAt = &t
	}
	if l.RevokedAt != nil {
		t := *l.RevokedAt
		c.RevokedAt = &t
	}
	if l.ExpiresAt != nil {
		t := *l.ExpiresAt
		c.ExpiresAt = &t
	}
	return &c
}

func cloneLocks(locks []*ActivationLock) []*ActivationLock {
	result := make([]*ActivationLock, len(locks))
	for i, l := range locks {
		result[i] = cloneLock(l)
	}
	return result
}

func cloneAuditEntry(e *AuditEntry) *AuditEntry {
	if e == nil {
		return nil
	}
	c := *e
	if e.Details != nil {
		c.Details = make(map[string]interface{})
		for k, v := range e.Details {
			c.Details[k] = v
		}
	}
	return &c
}

func cloneAuditEntries(entries []*AuditEntry) []*AuditEntry {
	result := make([]*AuditEntry, len(entries))
	for i, e := range entries {
		result[i] = cloneAuditEntry(e)
	}
	return result
}

func cloneVersion(v *SecretVersion) *SecretVersion {
	if v == nil {
		return nil
	}
	c := *v
	if v.RotatedAt != nil {
		t := *v.RotatedAt
		c.RotatedAt = &t
	}
	if v.Metadata != nil {
		c.Metadata = make(map[string]string)
		for k, val := range v.Metadata {
			c.Metadata[k] = val
		}
	}
	return &c
}

func cloneVersionHistory(vh *VersionHistory) *VersionHistory {
	if vh == nil {
		return nil
	}
	c := *vh
	if vh.Versions != nil {
		c.Versions = make([]*SecretVersion, len(vh.Versions))
		for i, v := range vh.Versions {
			c.Versions[i] = cloneVersion(v)
		}
	}
	if vh.LastRotation != nil {
		t := *vh.LastRotation
		c.LastRotation = &t
	}
	if vh.NextRotation != nil {
		t := *vh.NextRotation
		c.NextRotation = &t
	}
	if vh.Policy != nil {
		p := *vh.Policy
		if vh.Policy.NextRotation != nil {
			t := *vh.Policy.NextRotation
			p.NextRotation = &t
		}
		c.Policy = &p
	}
	return &c
}

func cloneVersionHistories(histories []*VersionHistory) []*VersionHistory {
	result := make([]*VersionHistory, len(histories))
	for i, vh := range histories {
		result[i] = cloneVersionHistory(vh)
	}
	return result
}
