package marketplace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu   sync.Mutex
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Store) MarketplaceDir() string {
	return filepath.Join(s.root, ".anyclaw", "marketplace")
}

func (s *Store) InstalledDir() string {
	return filepath.Join(s.MarketplaceDir(), "installed")
}

func (s *Store) ReceiptsDir() string {
	return filepath.Join(s.MarketplaceDir(), "receipts")
}

func (s *Store) JobsDir() string {
	return filepath.Join(s.MarketplaceDir(), "jobs")
}

func (s *Store) BindingsDir() string {
	return filepath.Join(s.MarketplaceDir(), "bindings")
}

func (s *Store) AuditDir() string {
	return filepath.Join(s.MarketplaceDir(), "audit")
}

func (s *Store) EventsDir() string {
	return filepath.Join(s.MarketplaceDir(), "events")
}

func (s *Store) Ensure() error {
	for _, dir := range []string{s.InstalledDir(), s.ReceiptsDir(), s.JobsDir(), s.BindingsDir(), s.AuditDir(), s.EventsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateInstallJob(req InstallRequest, idempotencyKey string) (*InstallJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return nil, false, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey != "" {
		index, err := s.loadIdempotencyIndex()
		if err != nil {
			return nil, false, err
		}
		if jobID := index[idempotencyKey]; jobID != "" {
			job, err := s.getJobLocked(jobID)
			if err == nil {
				return job, true, nil
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	job := &InstallJob{
		ID:                "market-job-" + time.Now().UTC().Format("20060102150405.000000000"),
		Type:              "install",
		State:             JobPending,
		ArtifactID:        strings.TrimSpace(req.ArtifactID),
		VersionConstraint: strings.TrimSpace(req.VersionConstraint),
		ProgressTotal:     5,
		IdempotencyKey:    idempotencyKey,
		InstalledBy:       firstNonEmpty(strings.TrimSpace(req.InstalledBy), "user"),
		Metadata: map[string]string{
			"user_confirmed":    marketBoolString(req.UserConfirmed),
			"risk_acknowledged": marketBoolString(req.RiskAcknowledged),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.saveJobLocked(job); err != nil {
		return nil, false, err
	}
	if idempotencyKey != "" {
		index, err := s.loadIdempotencyIndex()
		if err != nil {
			return nil, false, err
		}
		index[idempotencyKey] = job.ID
		if err := s.saveIdempotencyIndex(index); err != nil {
			return nil, false, err
		}
	}
	return job, false, nil
}

func (s *Store) CreateUpgradeJob(req UpgradeRequest, idempotencyKey string) (*InstallJob, bool, error) {
	installReq := InstallRequest{
		ArtifactID:        req.ArtifactID,
		VersionConstraint: req.VersionConstraint,
		InstalledBy:       req.InstalledBy,
		UserConfirmed:     req.UserConfirmed,
		RiskAcknowledged:  req.RiskAcknowledged,
		IdempotencyKey:    req.IdempotencyKey,
	}
	job, reused, err := s.CreateInstallJob(installReq, idempotencyKey)
	if err != nil || reused {
		return job, reused, err
	}
	job.Type = "upgrade"
	if job.Metadata == nil {
		job.Metadata = map[string]string{}
	}
	job.Metadata["previous_version"] = ""
	if previous, err := s.LatestReceiptForArtifact(req.ArtifactID); err == nil {
		job.Metadata["previous_version"] = previous.Version
		job.Metadata["previous_receipt_id"] = previous.ID
	}
	if err := s.UpdateJob(job); err != nil {
		return nil, false, err
	}
	return job, false, nil
}

func (s *Store) GetJob(id string) (*InstallJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getJobLocked(id)
}

func (s *Store) ListJobs(limit int) (JobListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return JobListResult{}, err
	}
	entries, err := os.ReadDir(s.JobsDir())
	if err != nil {
		return JobListResult{}, err
	}
	var items []InstallJob
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		job, err := s.getJobLocked(strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil {
			items = append(items, *job)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	if limit <= 0 {
		limit = 100
	}
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return JobListResult{Items: items, Total: total}, nil
}

func (s *Store) UpdateJob(job *InstallJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.saveJobLocked(job)
}

func (s *Store) SaveReceipt(receipt *InstallReceipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt == nil {
		return fmt.Errorf("receipt is nil")
	}
	if err := os.MkdirAll(s.ReceiptsDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.receiptPath(receipt.ID), data, 0o644)
}

func (s *Store) AppendAudit(event MarketAuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return err
	}
	if event.ID == "" {
		event.ID = "market-audit-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.auditPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) AppendEvent(event MarketEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return err
	}
	if event.ID == "" {
		event.ID = "market-event-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.eventPath(event.ID), data, 0o644)
}

func (s *Store) ListEvents(limit int) (MarketEventListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return MarketEventListResult{}, err
	}
	entries, err := os.ReadDir(s.EventsDir())
	if err != nil {
		return MarketEventListResult{}, err
	}
	var items []MarketEvent
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.EventsDir(), entry.Name()))
		if err != nil {
			continue
		}
		var event MarketEvent
		if err := json.Unmarshal(data, &event); err == nil && event.ID != "" {
			items = append(items, event)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	if limit <= 0 {
		limit = 100
	}
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return MarketEventListResult{Items: items, Total: total}, nil
}

func (s *Store) ListReceipts() ([]InstallReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.ReceiptsDir())
	if err != nil {
		return nil, err
	}
	var receipts []InstallReceipt
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.ReceiptsDir(), entry.Name()))
		if err != nil {
			continue
		}
		var receipt InstallReceipt
		if err := json.Unmarshal(data, &receipt); err == nil && receipt.ID != "" {
			receipts = append(receipts, receipt)
		}
	}
	return receipts, nil
}

func (s *Store) LatestReceiptForArtifact(artifactID string) (*InstallReceipt, error) {
	receipts, err := s.ListReceipts()
	if err != nil {
		return nil, err
	}
	var best *InstallReceipt
	for i := range receipts {
		if !strings.EqualFold(strings.TrimSpace(receipts[i].ArtifactID), strings.TrimSpace(artifactID)) {
			continue
		}
		if best == nil || receipts[i].InstalledAt > best.InstalledAt {
			copy := receipts[i]
			best = &copy
		}
	}
	if best == nil {
		return nil, ErrArtifactNotFound
	}
	return best, nil
}

func (s *Store) GetReceipt(id string) (*InstallReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getReceiptLocked(id)
}

func (s *Store) CreateBinding(req BindingRequest) (*Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	receiptID := strings.TrimSpace(req.ReceiptID)
	var receipt *InstallReceipt
	var err error
	if receiptID != "" {
		receipt, err = s.getReceiptLocked(receiptID)
	} else {
		receipt, err = s.latestReceiptForArtifactLocked(req.ArtifactID)
	}
	if err != nil {
		return nil, err
	}
	targetType := NormalizeBindingTargetType(string(req.TargetType))
	if targetType == "" {
		return nil, fmt.Errorf("invalid target_type")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	binding := &Binding{
		ID:         "binding-" + time.Now().UTC().Format("20060102150405.000000000"),
		ArtifactID: receipt.ArtifactID,
		ReceiptID:  receipt.ID,
		Kind:       receipt.Kind,
		Version:    receipt.Version,
		TargetType: targetType,
		TargetID:   strings.TrimSpace(req.TargetID),
		State:      BindingEnabled,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   cloneStringMap(req.Metadata),
	}
	if binding.TargetID == "" && targetType != TargetRuntimeGlobal {
		return nil, fmt.Errorf("target_id is required for %s", targetType)
	}
	if err := s.saveBindingLocked(binding); err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *Store) ListBindings() (BindingListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return BindingListResult{}, err
	}
	entries, err := os.ReadDir(s.BindingsDir())
	if err != nil {
		return BindingListResult{}, err
	}
	var items []Binding
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		binding, err := s.getBindingLocked(strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil {
			items = append(items, *binding)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return BindingListResult{Items: items, Total: len(items)}, nil
}

func (s *Store) DeleteBinding(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bindingPath(id)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrArtifactNotFound
		}
		return err
	}
	return nil
}

func (s *Store) DeleteBindingsForArtifact(artifactID string) ([]Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.BindingsDir())
	if err != nil {
		return nil, err
	}
	var removed []Binding
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		binding, err := s.getBindingLocked(id)
		if err != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(binding.ArtifactID), strings.TrimSpace(artifactID)) {
			continue
		}
		if err := os.Remove(s.bindingPath(binding.ID)); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed = append(removed, *binding)
	}
	return removed, nil
}

func (s *Store) UpdateBindingsForArtifactReceipt(artifactID, receiptID, version string) ([]Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.BindingsDir())
	if err != nil {
		return nil, err
	}
	var updated []Binding
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		binding, err := s.getBindingLocked(id)
		if err != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(binding.ArtifactID), strings.TrimSpace(artifactID)) {
			continue
		}
		binding.ReceiptID = receiptID
		binding.Version = version
		binding.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.saveBindingLocked(binding); err != nil {
			return updated, err
		}
		updated = append(updated, *binding)
	}
	return updated, nil
}

func (s *Store) DeleteReceipt(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.receiptPath(id)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrArtifactNotFound
		}
		return err
	}
	return nil
}

func (s *Store) OverlayStatus(items []Artifact) []Artifact {
	receipts, _ := s.ListReceipts()
	bindingsResult, _ := s.ListBindings()
	installed := map[string]InstallReceipt{}
	for _, receipt := range receipts {
		key := strings.ToLower(strings.TrimSpace(receipt.ArtifactID))
		if key == "" {
			continue
		}
		if current, ok := installed[key]; !ok || receipt.InstalledAt > current.InstalledAt {
			installed[key] = receipt
		}
	}
	bound := map[string]Binding{}
	active := map[string]Binding{}
	for _, binding := range bindingsResult.Items {
		if binding.State != BindingEnabled {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(binding.ArtifactID))
		bound[key] = binding
		if binding.TargetType == TargetMainAgent || binding.TargetType == TargetRuntimeGlobal {
			active[key] = binding
		}
	}
	out := append([]Artifact(nil), items...)
	for i := range out {
		key := strings.ToLower(strings.TrimSpace(out[i].ID))
		if receipt, ok := installed[key]; ok {
			out[i].Installed = true
			out[i].Enabled = true
			out[i].Status = StatusInstalled
			out[i].Version = firstNonEmpty(out[i].Version, receipt.Version)
		}
		if _, ok := bound[key]; ok {
			out[i].Bound = true
			out[i].Status = StatusBound
		}
		if _, ok := active[key]; ok {
			out[i].Active = true
			out[i].Status = StatusActive
		}
	}
	return out
}

func (s *Store) ReceiptPath(id string) string {
	return s.receiptPath(id)
}

func (s *Store) AuditPath() string {
	return s.auditPath()
}

func (s *Store) receiptPath(id string) string {
	return filepath.Join(s.ReceiptsDir(), safeName(id)+".json")
}

func (s *Store) auditPath() string {
	return filepath.Join(s.AuditDir(), "marketplace.jsonl")
}

func (s *Store) eventPath(id string) string {
	return filepath.Join(s.EventsDir(), safeName(id)+".json")
}

func (s *Store) getReceiptLocked(id string) (*InstallReceipt, error) {
	data, err := os.ReadFile(s.receiptPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrArtifactNotFound
		}
		return nil, err
	}
	var receipt InstallReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (s *Store) latestReceiptForArtifactLocked(artifactID string) (*InstallReceipt, error) {
	entries, err := os.ReadDir(s.ReceiptsDir())
	if err != nil {
		return nil, err
	}
	var best *InstallReceipt
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.ReceiptsDir(), entry.Name()))
		if err != nil {
			continue
		}
		var receipt InstallReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(receipt.ArtifactID), strings.TrimSpace(artifactID)) {
			if best == nil || receipt.InstalledAt > best.InstalledAt {
				copy := receipt
				best = &copy
			}
		}
	}
	if best == nil {
		return nil, ErrArtifactNotFound
	}
	return best, nil
}

func (s *Store) getBindingLocked(id string) (*Binding, error) {
	data, err := os.ReadFile(s.bindingPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrArtifactNotFound
		}
		return nil, err
	}
	var binding Binding
	if err := json.Unmarshal(data, &binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

func (s *Store) saveBindingLocked(binding *Binding) error {
	if binding == nil {
		return fmt.Errorf("binding is nil")
	}
	if err := os.MkdirAll(s.BindingsDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.bindingPath(binding.ID), data, 0o644)
}

func (s *Store) bindingPath(id string) string {
	return filepath.Join(s.BindingsDir(), safeName(id)+".json")
}

func (s *Store) getJobLocked(id string) (*InstallJob, error) {
	data, err := os.ReadFile(s.jobPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrArtifactNotFound
		}
		return nil, err
	}
	var job InstallJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) saveJobLocked(job *InstallJob) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	if err := os.MkdirAll(s.JobsDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.jobPath(job.ID), data, 0o644)
}

func (s *Store) jobPath(id string) string {
	return filepath.Join(s.JobsDir(), safeName(id)+".json")
}

func (s *Store) loadIdempotencyIndex() (map[string]string, error) {
	path := filepath.Join(s.MarketplaceDir(), "idempotency.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var index map[string]string
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index == nil {
		index = map[string]string{}
	}
	return index, nil
}

func (s *Store) saveIdempotencyIndex(index map[string]string) error {
	if err := os.MkdirAll(s.MarketplaceDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(s.MarketplaceDir(), "idempotency.json"), data, 0o644)
}

func safeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "item"
	}
	return value
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func marketBoolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
