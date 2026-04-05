package enterprise

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// TamperProofAudit implements an append-only audit log with cryptographic
// hash chain for tamper detection.
type TamperProofAudit struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	lastHash string
	entries  []*AuditEntry
	maxSize  int
}

// AuditEntry is a single tamper-proof audit log entry.
type AuditEntry struct {
	Index      int64          `json:"index"`
	Timestamp  time.Time      `json:"timestamp"`
	Actor      string         `json:"actor"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	Details    map[string]any `json:"details,omitempty"`
	PrevHash   string         `json:"prev_hash"`
	EntryHash  string         `json:"entry_hash"`
	Correlated string         `json:"correlated,omitempty"`
}

// AuditIntegrityReport reports on the integrity of the audit log.
type AuditIntegrityReport struct {
	TotalEntries    int       `json:"total_entries"`
	ValidEntries    int       `json:"valid_entries"`
	TamperedEntries []int64   `json:"tampered_entries,omitempty"`
	FirstHash       string    `json:"first_hash"`
	LastHash        string    `json:"last_hash"`
	ChainIntact     bool      `json:"chain_intact"`
	CheckedAt       time.Time `json:"checked_at"`
}

// NewTamperProofAudit creates a new tamper-proof audit log.
func NewTamperProofAudit(path string) (*TamperProofAudit, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	audit := &TamperProofAudit{
		file:    f,
		path:    path,
		maxSize: 100000,
	}

	if err := audit.loadExisting(); err != nil {
		_ = f.Close()
		return nil, err
	}

	return audit, nil
}

func (a *TamperProofAudit) loadExisting() error {
	data, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := splitLines(string(data))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		a.entries = append(a.entries, &entry)
		a.lastHash = entry.EntryHash
	}

	return nil
}

// Append adds a new entry to the audit log with hash chain linkage.
func (a *TamperProofAudit) Append(actor, action, resource string, details map[string]any) (*AuditEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	index := int64(len(a.entries))

	entry := &AuditEntry{
		Index:     index,
		Timestamp: time.Now().UTC(),
		Actor:     actor,
		Action:    action,
		Resource:  resource,
		Details:   details,
		PrevHash:  a.lastHash,
	}

	entry.EntryHash = a.computeEntryHash(entry)
	a.lastHash = entry.EntryHash
	a.entries = append(a.entries, entry)

	line, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	if _, err := a.file.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write audit entry: %w", err)
	}

	if err := a.file.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync audit log: %w", err)
	}

	return entry, nil
}

// AppendWithCorrelation adds an entry with a correlation ID for cross-system tracing.
func (a *TamperProofAudit) AppendWithCorrelation(correlationID, actor, action, resource string, details map[string]any) (*AuditEntry, error) {
	if details == nil {
		details = make(map[string]any)
	}
	details["correlation_id"] = correlationID
	return a.Append(actor, action, resource, details)
}

func (a *TamperProofAudit) computeEntryHash(entry *AuditEntry) string {
	hashable := fmt.Sprintf("%d|%s|%s|%s|%s|%s",
		entry.Index,
		entry.Timestamp.Format(time.RFC3339Nano),
		entry.Actor,
		entry.Action,
		entry.Resource,
		entry.PrevHash,
	)

	if entry.Details != nil {
		detailsJSON, _ := json.Marshal(entry.Details)
		hashable += "|" + string(detailsJSON)
	}

	sum := sha256.Sum256([]byte(hashable))
	return hex.EncodeToString(sum[:])
}

// VerifyIntegrity checks the entire hash chain for tampering.
func (a *TamperProofAudit) VerifyIntegrity() *AuditIntegrityReport {
	a.mu.Lock()
	defer a.mu.Unlock()

	report := &AuditIntegrityReport{
		TotalEntries:    len(a.entries),
		ValidEntries:    0,
		TamperedEntries: make([]int64, 0),
		ChainIntact:     true,
		CheckedAt:       time.Now().UTC(),
	}

	if len(a.entries) == 0 {
		return report
	}

	report.FirstHash = a.entries[0].EntryHash
	report.LastHash = a.entries[len(a.entries)-1].EntryHash

	var expectedHash string
	for _, entry := range a.entries {
		computedHash := a.computeEntryHash(entry)

		if computedHash != entry.EntryHash {
			report.TamperedEntries = append(report.TamperedEntries, entry.Index)
			report.ChainIntact = false
			continue
		}

		if entry.PrevHash != expectedHash && entry.Index > 0 {
			report.TamperedEntries = append(report.TamperedEntries, entry.Index)
			report.ChainIntact = false
			continue
		}

		report.ValidEntries++
		expectedHash = entry.EntryHash
	}

	return report
}

// Query returns entries matching the given criteria.
func (a *TamperProofAudit) Query(actor, action, resource string, limit int) []*AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()

	var results []*AuditEntry
	for i := len(a.entries) - 1; i >= 0 && len(results) < limit; i-- {
		entry := a.entries[i]
		if actor != "" && entry.Actor != actor {
			continue
		}
		if action != "" && entry.Action != action {
			continue
		}
		if resource != "" && entry.Resource != resource {
			continue
		}
		results = append(results, entry)
	}

	return results
}

// GetEntry returns a specific entry by index.
func (a *TamperProofAudit) GetEntry(index int64) (*AuditEntry, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if index < 0 || index >= int64(len(a.entries)) {
		return nil, false
	}
	return a.entries[index], true
}

// Close closes the audit log file.
func (a *TamperProofAudit) Close() error {
	return a.file.Close()
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
