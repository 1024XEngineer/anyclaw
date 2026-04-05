package enterprise

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ComplianceManager handles GDPR/HIPAA compliance requirements including
// data retention, PII detection, DSAR (Data Subject Access Requests),
// and right-to-erasure.
type ComplianceManager struct {
	mu           sync.RWMutex
	policies     []RetentionPolicy
	piiPatterns  []*regexp.Regexp
	dataSubjects map[string]*DataSubjectRecord
	dsarLog      []DSARRecord
	erasureLog   []ErasureRecord
}

// RetentionPolicy defines how long data should be retained.
type RetentionPolicy struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Category    string        `json:"category"` // "pii", "audit", "session", "memory", "general"
	Retention   time.Duration `json:"retention"`
	Compliance  string        `json:"compliance"` // "gdpr", "hipaa", "sox", "internal"
	AutoDelete  bool          `json:"auto_delete"`
	CreatedAt   time.Time     `json:"created_at"`
}

// DataSubjectRecord tracks personal data for a data subject.
type DataSubjectRecord struct {
	SubjectID   string     `json:"subject_id"`
	DataType    string     `json:"data_type"`
	Location    string     `json:"location"`
	CollectedAt time.Time  `json:"collected_at"`
	LegalBasis  string     `json:"legal_basis"` // "consent", "contract", "legitimate_interest", "legal_obligation"
	ConsentDate *time.Time `json:"consent_date,omitempty"`
	Fields      []string   `json:"fields"`
	Encrypted   bool       `json:"encrypted"`
}

// DSARRecord tracks a Data Subject Access Request.
type DSARRecord struct {
	ID          string     `json:"id"`
	SubjectID   string     `json:"subject_id"`
	Type        string     `json:"type"` // "access", "erasure", "rectification", "portability", "restriction"
	RequestedAt time.Time  `json:"requested_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Status      string     `json:"status"` // "pending", "in_progress", "completed", "rejected"
	Details     string     `json:"details,omitempty"`
	Data        []byte     `json:"data,omitempty"`
}

// ErasureRecord tracks a data erasure operation.
type ErasureRecord struct {
	ID         string    `json:"id"`
	SubjectID  string    `json:"subject_id"`
	DataType   string    `json:"data_type"`
	Location   string    `json:"location"`
	ErasedAt   time.Time `json:"erased_at"`
	Method     string    `json:"method"` // "delete", "anonymize", "pseudonymize"
	VerifiedBy string    `json:"verified_by"`
}

// NewComplianceManager creates a new compliance manager.
func NewComplianceManager() *ComplianceManager {
	cm := &ComplianceManager{
		dataSubjects: make(map[string]*DataSubjectRecord),
		dsarLog:      make([]DSARRecord, 0),
		erasureLog:   make([]ErasureRecord, 0),
	}

	// Default PII patterns
	cm.piiPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`), // email
		regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                               // SSN
		regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),          // credit card
		regexp.MustCompile(`\b\d{3}[\s-]?\d{3}[\s-]?\d{4}\b`),                     // phone
		regexp.MustCompile(`\b\d{9}\b`),                                           // national ID
		regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{4}\d{7}([A-Z0-9]?){0,16}\b`),  // IBAN
	}

	// Default retention policies
	cm.policies = []RetentionPolicy{
		{
			ID:         "gdpr-consent",
			Name:       "GDPR Consent Records",
			Category:   "pii",
			Retention:  3 * 365 * 24 * time.Hour,
			Compliance: "gdpr",
			AutoDelete: true,
		},
		{
			ID:         "gdpr-session",
			Name:       "GDPR Session Data",
			Category:   "session",
			Retention:  30 * 24 * time.Hour,
			Compliance: "gdpr",
			AutoDelete: true,
		},
		{
			ID:         "hipaa-audit",
			Name:       "HIPAA Audit Logs",
			Category:   "audit",
			Retention:  6 * 365 * 24 * time.Hour,
			Compliance: "hipaa",
			AutoDelete: false,
		},
		{
			ID:         "gdpr-audit",
			Name:       "GDPR Audit Logs",
			Category:   "audit",
			Retention:  2 * 365 * 24 * time.Hour,
			Compliance: "gdpr",
			AutoDelete: false,
		},
		{
			ID:         "internal-memory",
			Name:       "Internal Memory Data",
			Category:   "memory",
			Retention:  90 * 24 * time.Hour,
			Compliance: "internal",
			AutoDelete: true,
		},
	}

	return cm
}

// AddRetentionPolicy adds a new retention policy.
func (cm *ComplianceManager) AddRetentionPolicy(policy RetentionPolicy) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	policy.CreatedAt = time.Now().UTC()
	cm.policies = append(cm.policies, policy)
}

// GetRetentionPolicies returns all retention policies.
func (cm *ComplianceManager) GetRetentionPolicies() []RetentionPolicy {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return append([]RetentionPolicy{}, cm.policies...)
}

// GetPolicyForCategory returns the retention policy for a category.
func (cm *ComplianceManager) GetPolicyForCategory(category, compliance string) *RetentionPolicy {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, p := range cm.policies {
		if p.Category == category && p.Compliance == compliance {
			return &p
		}
	}
	return nil
}

// RegisterDataSubject registers personal data for a data subject.
func (cm *ComplianceManager) RegisterDataSubject(record DataSubjectRecord) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	key := record.SubjectID + ":" + record.DataType + ":" + record.Location
	cm.dataSubjects[key] = &record
}

// GetDataSubjectRecords returns all records for a data subject.
func (cm *ComplianceManager) GetDataSubjectRecords(subjectID string) []*DataSubjectRecord {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var records []*DataSubjectRecord
	for key, record := range cm.dataSubjects {
		if strings.HasPrefix(key, subjectID+":") {
			records = append(records, record)
		}
	}
	return records
}

// DetectPII scans text for personally identifiable information.
func (cm *ComplianceManager) DetectPII(text string) []PIIFinding {
	var findings []PIIFinding
	for _, pattern := range cm.piiPatterns {
		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			findings = append(findings, PIIFinding{
				Type:     classifyPII(pattern),
				Value:    maskPII(text[match[0]:match[1]]),
				Start:    match[0],
				End:      match[1],
				Original: text[match[0]:match[1]],
			})
		}
	}
	return findings
}

// PIIFinding represents a detected PII element.
type PIIFinding struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Original string `json:"-"`
}

// RedactPII replaces PII in text with masked values.
func (cm *ComplianceManager) RedactPII(text string) string {
	findings := cm.DetectPII(text)
	if len(findings) == 0 {
		return text
	}

	result := text
	for i := len(findings) - 1; i >= 0; i-- {
		f := findings[i]
		result = result[:f.Start] + f.Value + result[f.End:]
	}
	return result
}

// SubmitDSAR submits a Data Subject Access Request.
func (cm *ComplianceManager) SubmitDSAR(subjectID, dsarType, details string) (*DSARRecord, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	record := DSARRecord{
		ID:          fmt.Sprintf("dsar_%d", time.Now().UnixNano()),
		SubjectID:   subjectID,
		Type:        dsarType,
		RequestedAt: time.Now().UTC(),
		Status:      "pending",
		Details:     details,
	}

	cm.dsarLog = append(cm.dsarLog, record)
	return &record, nil
}

// CompleteDSAR marks a DSAR as completed.
func (cm *ComplianceManager) CompleteDSAR(dsarID string, data []byte) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i := range cm.dsarLog {
		if cm.dsarLog[i].ID == dsarID {
			now := time.Now().UTC()
			cm.dsarLog[i].CompletedAt = &now
			cm.dsarLog[i].Status = "completed"
			cm.dsarLog[i].Data = data
			return nil
		}
	}
	return fmt.Errorf("DSAR not found: %s", dsarID)
}

// GetDSARs returns DSARs for a subject.
func (cm *ComplianceManager) GetDSARs(subjectID string) []DSARRecord {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var results []DSARRecord
	for _, r := range cm.dsarLog {
		if subjectID == "" || r.SubjectID == subjectID {
			results = append(results, r)
		}
	}
	return results
}

// RecordErasure logs a data erasure operation.
func (cm *ComplianceManager) RecordErasure(record ErasureRecord) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	record.ErasedAt = time.Now().UTC()
	cm.erasureLog = append(cm.erasureLog, record)
}

// GetErasureLog returns the erasure log.
func (cm *ComplianceManager) GetErasureLog() []ErasureRecord {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return append([]ErasureRecord{}, cm.erasureLog...)
}

// CheckExpiredData returns data that has exceeded its retention period.
func (cm *ComplianceManager) CheckExpiredData() []ExpiredDataItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var expired []ExpiredDataItem
	now := time.Now().UTC()

	for key, record := range cm.dataSubjects {
		policy := cm.getPolicyForRecord(record)
		if policy == nil || !policy.AutoDelete {
			continue
		}

		expiryDate := record.CollectedAt.Add(policy.Retention)
		if now.After(expiryDate) {
			expired = append(expired, ExpiredDataItem{
				Key:        key,
				SubjectID:  record.SubjectID,
				DataType:   record.DataType,
				Location:   record.Location,
				ExpiredAt:  expiryDate,
				PolicyName: policy.Name,
			})
		}
	}

	return expired
}

// ExportComplianceReport generates a compliance report.
func (cm *ComplianceManager) ExportComplianceReport(mode string) (map[string]any, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	report := map[string]any{
		"mode":               mode,
		"generated_at":       time.Now().UTC(),
		"retention_policies": cm.policies,
		"data_subjects":      len(cm.dataSubjects),
		"dsar_count":         len(cm.dsarLog),
		"erasure_count":      len(cm.erasureLog),
	}

	if mode == "gdpr" {
		gdprPolicies := make([]RetentionPolicy, 0)
		for _, p := range cm.policies {
			if p.Compliance == "gdpr" {
				gdprPolicies = append(gdprPolicies, p)
			}
		}
		report["gdpr_policies"] = gdprPolicies
		report["gdpr_dsars"] = cm.dsarLog
	}

	if mode == "hipaa" {
		hipaaPolicies := make([]RetentionPolicy, 0)
		for _, p := range cm.policies {
			if p.Compliance == "hipaa" {
				hipaaPolicies = append(hipaaPolicies, p)
			}
		}
		report["hipaa_policies"] = hipaaPolicies
	}

	return report, nil
}

// ExpiredDataItem represents data that has exceeded its retention period.
type ExpiredDataItem struct {
	Key        string    `json:"key"`
	SubjectID  string    `json:"subject_id"`
	DataType   string    `json:"data_type"`
	Location   string    `json:"location"`
	ExpiredAt  time.Time `json:"expired_at"`
	PolicyName string    `json:"policy_name"`
}

func (cm *ComplianceManager) getPolicyForRecord(record *DataSubjectRecord) *RetentionPolicy {
	for _, p := range cm.policies {
		if p.Category == record.DataType {
			return &p
		}
	}
	return nil
}

func classifyPII(pattern *regexp.Regexp) string {
	switch pattern.String() {
	case `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`:
		return "email"
	case `\b\d{3}-\d{2}-\d{4}\b`:
		return "ssn"
	case `\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`:
		return "credit_card"
	case `\b\d{3}[\s-]?\d{3}[\s-]?\d{4}\b`:
		return "phone"
	case `\b[A-Z]{2}\d{2}[A-Z0-9]{4}\d{7}([A-Z0-9]?){0,16}\b`:
		return "iban"
	default:
		return "unknown"
	}
}

func maskPII(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

// SaveComplianceConfig saves the compliance configuration to a file.
func (cm *ComplianceManager) SaveComplianceConfig(path string) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	config := map[string]any{
		"policies":      cm.policies,
		"pii_patterns":  len(cm.piiPatterns),
		"data_subjects": len(cm.dataSubjects),
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
