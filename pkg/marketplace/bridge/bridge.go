package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	marketregistry "github.com/1024XEngineer/anyclaw/pkg/marketplace/registry"
)

type Bridge interface {
	Search(ctx context.Context, req SearchRequest) (SearchResult, error)
	List(ctx context.Context, filter marketplace.Filter) (ListResult, error)
	Get(ctx context.Context, artifactID string, source marketplace.SourceKind) (*marketplace.Artifact, error)
	Versions(ctx context.Context, artifactID string, source marketplace.SourceKind) ([]marketplace.ArtifactVersion, error)
	Resolve(ctx context.Context, artifactID, versionConstraint string) (marketplace.ResolvedPackage, error)
	PlanInstall(ctx context.Context, req marketplace.InstallRequest) (InstallPlan, error)
	StartInstall(ctx context.Context, req marketplace.InstallRequest) (InstallResult, error)
	Install(ctx context.Context, req marketplace.InstallRequest) (InstallResult, error)
	StartUpgrade(ctx context.Context, req marketplace.UpgradeRequest) (InstallResult, error)
	ExecuteJob(ctx context.Context, jobID string) (*marketplace.InstallJob, error)
	ListJobs(limit int) (marketplace.JobListResult, error)
	GetJob(jobID string) (*marketplace.InstallJob, error)
	Bind(ctx context.Context, req marketplace.BindingRequest) (*marketplace.Binding, error)
	ListBindings() (marketplace.BindingListResult, error)
	DeleteBinding(ctx context.Context, bindingID string) (*marketplace.Binding, error)
	Uninstall(ctx context.Context, req marketplace.UninstallRequest) (*marketplace.UninstallResult, error)
	ListEvents(limit int) (marketplace.MarketEventListResult, error)
}

type SearchRequest struct {
	Query      string
	Kind       marketplace.ArtifactKind
	Source     marketplace.SourceKind
	SearchMode marketplace.SearchMode
	Limit      int
}

type SearchResult struct {
	Local         []marketplace.Artifact
	Cloud         []marketplace.Artifact
	CloudErr      string
	RetrievalMeta *marketplace.RetrievalMeta
}

type ListResult struct {
	Result   marketplace.ListResult
	CloudErr string
}

type InstallResult struct {
	Job    *marketplace.InstallJob
	Reused bool
}

type InstallPlan struct {
	Request  marketplace.InstallRequest  `json:"request"`
	Artifact marketplace.ResolvedPackage `json:"artifact"`
	Decision marketplace.PolicyDecision  `json:"decision"`
}

type Options struct {
	Store            *marketplace.Store
	Registry         *marketregistry.Client
	LocalCatalog     *marketplace.LocalCatalog
	AutoInstallSkill bool
	AfterInstall     func(context.Context, *marketplace.InstallReceipt) error
	AfterBind        func(context.Context, *marketplace.Binding) error
	BeforeUninstall  func(context.Context, *marketplace.InstallReceipt) error
	AfterUninstall   func(context.Context, *marketplace.UninstallResult) error
}

type DefaultBridge struct {
	store            *marketplace.Store
	registry         *marketregistry.Client
	localCatalog     *marketplace.LocalCatalog
	autoInstallSkill bool
	afterInstall     func(context.Context, *marketplace.InstallReceipt) error
	afterBind        func(context.Context, *marketplace.Binding) error
	beforeUninstall  func(context.Context, *marketplace.InstallReceipt) error
	afterUninstall   func(context.Context, *marketplace.UninstallResult) error
}

func New(opts Options) *DefaultBridge {
	return &DefaultBridge{
		store:            opts.Store,
		registry:         opts.Registry,
		localCatalog:     opts.LocalCatalog,
		autoInstallSkill: opts.AutoInstallSkill,
		afterInstall:     opts.AfterInstall,
		afterBind:        opts.AfterBind,
		beforeUninstall:  opts.BeforeUninstall,
		afterUninstall:   opts.AfterUninstall,
	}
}

func (b *DefaultBridge) Search(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if b == nil || b.store == nil {
		return SearchResult{}, fmt.Errorf("marketplace bridge store is not configured")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	local, err := localArtifacts(b.store, req.Kind, limit)
	if err != nil {
		return SearchResult{}, err
	}
	var cloud []marketplace.Artifact
	var cloudErr string
	var retrievalMeta *marketplace.RetrievalMeta
	if req.Source != marketplace.SourceLocal && b.registry != nil {
		result, err := b.registry.List(ctx, marketplace.Filter{
			Kind:       req.Kind,
			Query:      req.Query,
			SearchMode: req.SearchMode,
			Limit:      limit,
		})
		if err != nil {
			cloudErr = err.Error()
		} else {
			cloud = result.Items
			retrievalMeta = result.RetrievalMeta
		}
	}
	if req.Source == marketplace.SourceCloud {
		local = nil
	}
	return SearchResult{
		Local:         local,
		Cloud:         cloud,
		CloudErr:      cloudErr,
		RetrievalMeta: retrievalMeta,
	}, nil
}

func (b *DefaultBridge) List(ctx context.Context, filter marketplace.Filter) (ListResult, error) {
	if filter.Source == marketplace.SourceCloud {
		result, cloudErr := b.listCloud(ctx, filter)
		return ListResult{Result: result, CloudErr: cloudErr}, nil
	}
	if b == nil || b.localCatalog == nil {
		return ListResult{}, fmt.Errorf("marketplace local catalog is not configured")
	}
	result, err := b.localCatalog.List(ctx, filter)
	if err != nil {
		return ListResult{}, err
	}
	result.Items = b.overlayStatus(result.Items)
	return ListResult{Result: result}, nil
}

func (b *DefaultBridge) Get(ctx context.Context, artifactID string, source marketplace.SourceKind) (*marketplace.Artifact, error) {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return nil, marketplace.ErrArtifactNotFound
	}
	if source == marketplace.SourceCloud {
		if b == nil || b.registry == nil {
			return nil, marketregistry.ErrNotConfigured
		}
		artifact, err := b.registry.Get(ctx, artifactID)
		if err != nil {
			return nil, err
		}
		items := b.overlayStatus([]marketplace.Artifact{*artifact})
		if len(items) > 0 {
			artifact = &items[0]
		}
		return artifact, nil
	}
	if b == nil || b.localCatalog == nil {
		return nil, fmt.Errorf("marketplace local catalog is not configured")
	}
	artifact, err := b.localCatalog.Get(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	items := b.overlayStatus([]marketplace.Artifact{*artifact})
	if len(items) > 0 {
		artifact = &items[0]
	}
	return artifact, nil
}

func (b *DefaultBridge) Versions(ctx context.Context, artifactID string, source marketplace.SourceKind) ([]marketplace.ArtifactVersion, error) {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return nil, marketplace.ErrArtifactNotFound
	}
	if source == marketplace.SourceCloud {
		if b == nil || b.registry == nil {
			return nil, marketregistry.ErrNotConfigured
		}
		return b.registry.Versions(ctx, artifactID)
	}
	if b == nil || b.localCatalog == nil {
		return nil, fmt.Errorf("marketplace local catalog is not configured")
	}
	return b.localCatalog.Versions(ctx, artifactID)
}

func (b *DefaultBridge) Resolve(ctx context.Context, artifactID, versionConstraint string) (marketplace.ResolvedPackage, error) {
	if b == nil || b.registry == nil {
		return marketplace.ResolvedPackage{}, marketregistry.ErrNotConfigured
	}
	resolved, err := b.registry.Resolve(ctx, strings.TrimSpace(artifactID), marketregistry.ResolveRequest{VersionConstraint: strings.TrimSpace(versionConstraint)})
	if err != nil {
		return marketplace.ResolvedPackage{}, err
	}
	return resolvedPackage(resolved), nil
}

func (b *DefaultBridge) PlanInstall(ctx context.Context, req marketplace.InstallRequest) (InstallPlan, error) {
	req.ArtifactID = strings.TrimSpace(req.ArtifactID)
	req.VersionConstraint = strings.TrimSpace(req.VersionConstraint)
	if req.ArtifactID == "" {
		return InstallPlan{}, fmt.Errorf("artifact_id is required")
	}
	resolved, err := b.Resolve(ctx, req.ArtifactID, req.VersionConstraint)
	if err != nil {
		return InstallPlan{}, err
	}
	decision := b.decideInstall(req, resolved)
	return InstallPlan{Request: req, Artifact: resolved, Decision: decision}, nil
}

func (b *DefaultBridge) Install(ctx context.Context, req marketplace.InstallRequest) (InstallResult, error) {
	result, err := b.StartInstall(ctx, req)
	if err != nil {
		return result, err
	}
	if result.Reused || result.Job == nil {
		return result, nil
	}
	latest, err := b.ExecuteJob(ctx, result.Job.ID)
	if err != nil {
		return InstallResult{Job: latest, Reused: result.Reused}, err
	}
	return InstallResult{Job: latest, Reused: result.Reused}, nil
}

func (b *DefaultBridge) StartInstall(ctx context.Context, req marketplace.InstallRequest) (InstallResult, error) {
	if b == nil || b.store == nil || b.registry == nil {
		return InstallResult{}, fmt.Errorf("marketplace install is not configured")
	}
	uc := marketplace.NewInstallUseCaseWithPolicy(b.store, registryAdapter{client: b.registry}, marketplace.PolicyConfig{AutoInstallSkill: b.autoInstallSkill})
	job, reused, err := uc.Start(ctx, req)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Job: job, Reused: reused}, nil
}

func (b *DefaultBridge) StartUpgrade(ctx context.Context, req marketplace.UpgradeRequest) (InstallResult, error) {
	if b == nil || b.store == nil || b.registry == nil {
		return InstallResult{}, fmt.Errorf("marketplace upgrade is not configured")
	}
	job, reused, err := b.store.CreateUpgradeJob(req, req.IdempotencyKey)
	if err != nil {
		return InstallResult{}, err
	}
	if !reused {
		_ = b.store.AppendAudit(marketplace.MarketAuditEvent{
			Type:       "market.upgrade.started",
			ArtifactID: job.ArtifactID,
			JobID:      job.ID,
			Actor:      firstNonEmpty(req.InstalledBy, "user"),
			Detail: map[string]any{
				"version_constraint": req.VersionConstraint,
				"previous_version":   job.Metadata["previous_version"],
			},
		})
		_ = b.store.AppendEvent(marketplace.MarketEvent{
			Type:       "market.upgrade.started",
			Level:      "info",
			Message:    "Marketplace upgrade started",
			ArtifactID: job.ArtifactID,
			JobID:      job.ID,
			Payload: map[string]any{
				"version_constraint": req.VersionConstraint,
				"previous_version":   job.Metadata["previous_version"],
			},
		})
	}
	return InstallResult{Job: job, Reused: reused}, nil
}

func (b *DefaultBridge) ExecuteJob(ctx context.Context, jobID string) (*marketplace.InstallJob, error) {
	if b == nil || b.store == nil || b.registry == nil {
		return nil, fmt.Errorf("marketplace install is not configured")
	}
	uc := marketplace.NewInstallUseCaseWithPolicy(b.store, registryAdapter{client: b.registry}, marketplace.PolicyConfig{AutoInstallSkill: b.autoInstallSkill})
	if err := uc.Execute(ctx, strings.TrimSpace(jobID)); err != nil {
		latest, _ := b.store.GetJob(strings.TrimSpace(jobID))
		return latest, err
	}
	latest, err := b.store.GetJob(strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	if err := b.afterSuccessfulInstall(ctx, latest); err != nil {
		return latest, err
	}
	return latest, nil
}

func (b *DefaultBridge) ListJobs(limit int) (marketplace.JobListResult, error) {
	if b == nil || b.store == nil {
		return marketplace.JobListResult{}, fmt.Errorf("marketplace bridge store is not configured")
	}
	return b.store.ListJobs(limit)
}

func (b *DefaultBridge) GetJob(jobID string) (*marketplace.InstallJob, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("marketplace bridge store is not configured")
	}
	return b.store.GetJob(strings.TrimSpace(jobID))
}

func (b *DefaultBridge) Bind(ctx context.Context, req marketplace.BindingRequest) (*marketplace.Binding, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("marketplace bridge store is not configured")
	}
	binding, err := b.store.CreateBinding(req)
	if err != nil {
		return nil, err
	}
	if b.afterBind != nil {
		if err := b.afterBind(ctx, binding); err != nil {
			return binding, err
		}
	}
	_ = b.store.AppendAudit(marketplace.MarketAuditEvent{
		Type:       "market.agent_bind.succeeded",
		ArtifactID: binding.ArtifactID,
		BindingID:  binding.ID,
		Actor:      "agent",
		Detail: map[string]any{
			"target_type": binding.TargetType,
			"target_id":   binding.TargetID,
			"version":     binding.Version,
		},
	})
	return binding, nil
}

func (b *DefaultBridge) ListBindings() (marketplace.BindingListResult, error) {
	if b == nil || b.store == nil {
		return marketplace.BindingListResult{}, fmt.Errorf("marketplace bridge store is not configured")
	}
	return b.store.ListBindings()
}

func (b *DefaultBridge) DeleteBinding(ctx context.Context, bindingID string) (*marketplace.Binding, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("marketplace bridge store is not configured")
	}
	binding := b.findBinding(bindingID)
	if err := b.store.DeleteBinding(strings.TrimSpace(bindingID)); err != nil {
		return nil, err
	}
	if binding != nil && b.afterBind != nil {
		if err := b.afterBind(ctx, binding); err != nil {
			return binding, err
		}
	}
	_ = b.store.AppendAudit(marketplace.MarketAuditEvent{
		Type:      "market.binding.deleted",
		BindingID: strings.TrimSpace(bindingID),
		Actor:     "user",
	})
	_ = b.store.AppendEvent(marketplace.MarketEvent{
		Type:      "market.binding.deleted",
		Level:     "info",
		Message:   "Marketplace binding deleted",
		BindingID: strings.TrimSpace(bindingID),
	})
	return binding, nil
}

func (b *DefaultBridge) Uninstall(ctx context.Context, req marketplace.UninstallRequest) (*marketplace.UninstallResult, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("marketplace bridge store is not configured")
	}
	receipt, err := b.uninstallReceipt(req)
	if err != nil {
		return nil, err
	}
	if b.beforeUninstall != nil {
		if err := b.beforeUninstall(ctx, receipt); err != nil {
			return nil, err
		}
	}
	result, err := marketplace.NewLifecycleService(b.store).Uninstall(req)
	if err != nil {
		return nil, err
	}
	if b.afterUninstall != nil {
		if err := b.afterUninstall(ctx, result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (b *DefaultBridge) ListEvents(limit int) (marketplace.MarketEventListResult, error) {
	if b == nil || b.store == nil {
		return marketplace.MarketEventListResult{}, fmt.Errorf("marketplace bridge store is not configured")
	}
	return b.store.ListEvents(limit)
}

func (b *DefaultBridge) decideInstall(req marketplace.InstallRequest, resolved marketplace.ResolvedPackage) marketplace.PolicyDecision {
	autoInstall := false
	if b != nil {
		autoInstall = b.autoInstallSkill
	}
	return marketplace.NewDecisionPolicy(marketplace.PolicyConfig{AutoInstallSkill: autoInstall}).DecideInstall(req, resolved)
}

func (b *DefaultBridge) afterSuccessfulInstall(ctx context.Context, job *marketplace.InstallJob) error {
	if b == nil || b.store == nil || b.afterInstall == nil || job == nil || job.State != marketplace.JobSucceeded || strings.TrimSpace(job.ReceiptID) == "" {
		return nil
	}
	receipt, err := b.store.GetReceipt(job.ReceiptID)
	if err != nil {
		return err
	}
	return b.afterInstall(ctx, receipt)
}

func (b *DefaultBridge) findBinding(bindingID string) *marketplace.Binding {
	if b == nil || b.store == nil || strings.TrimSpace(bindingID) == "" {
		return nil
	}
	result, err := b.store.ListBindings()
	if err != nil {
		return nil
	}
	for i := range result.Items {
		if strings.EqualFold(strings.TrimSpace(result.Items[i].ID), strings.TrimSpace(bindingID)) {
			return &result.Items[i]
		}
	}
	return nil
}

func (b *DefaultBridge) uninstallReceipt(req marketplace.UninstallRequest) (*marketplace.InstallReceipt, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("marketplace bridge store is not configured")
	}
	if receiptID := strings.TrimSpace(req.ReceiptID); receiptID != "" {
		return b.store.GetReceipt(receiptID)
	}
	return b.store.LatestReceiptForArtifact(strings.TrimSpace(req.ArtifactID))
}

func (b *DefaultBridge) listCloud(ctx context.Context, filter marketplace.Filter) (marketplace.ListResult, string) {
	if b == nil || b.registry == nil {
		return emptyList(filter), "cloud registry endpoint is not configured"
	}
	cloudFilter := filter
	cloudFilter.Source = ""
	result, err := b.registry.List(ctx, cloudFilter)
	if err != nil {
		return emptyList(filter), err.Error()
	}
	result.Items = b.overlayStatus(result.Items)
	applyStatusFilter(&result, filter.Status)
	return result, ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (b *DefaultBridge) overlayStatus(items []marketplace.Artifact) []marketplace.Artifact {
	if b == nil || b.store == nil || len(items) == 0 {
		return items
	}
	return b.store.OverlayStatus(items)
}

func localArtifacts(store *marketplace.Store, kind marketplace.ArtifactKind, limit int) ([]marketplace.Artifact, error) {
	receipts, err := store.ListReceipts()
	if err != nil {
		return nil, err
	}
	items := make([]marketplace.Artifact, 0, len(receipts))
	for _, receipt := range receipts {
		if kind != "" && receipt.Kind != kind {
			continue
		}
		items = append(items, marketplace.Artifact{
			ID:            receipt.ArtifactID,
			Kind:          receipt.Kind,
			Name:          receipt.Name,
			DisplayName:   receipt.Name,
			Version:       receipt.Version,
			Source:        marketplace.SourceLocal,
			Status:        marketplace.StatusInstalled,
			Installed:     true,
			Enabled:       true,
			Permissions:   append([]string(nil), receipt.Permissions...),
			RiskLevel:     receipt.RiskLevel,
			TrustLevel:    receipt.TrustLevel,
			Compatibility: receipt.Compatibility,
			Dependencies:  append([]marketplace.ArtifactDependency(nil), receipt.Dependencies...),
			Capabilities:  []string{receipt.Name, string(receipt.Kind)},
		})
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

func emptyList(filter marketplace.Filter) marketplace.ListResult {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	return marketplace.ListResult{Items: []marketplace.Artifact{}, Total: 0, Limit: limit, Offset: offset}
}

func applyStatusFilter(result *marketplace.ListResult, status marketplace.ArtifactStatus) {
	if result == nil || status == "" {
		return
	}
	items := result.Items[:0]
	for _, item := range result.Items {
		if item.Status == status {
			items = append(items, item)
		}
	}
	result.Items = items
	result.Total = len(items)
}

type registryAdapter struct {
	client *marketregistry.Client
}

func (a registryAdapter) Resolve(ctx context.Context, artifactID, versionConstraint string) (marketplace.ResolvedPackage, error) {
	if a.client == nil {
		return marketplace.ResolvedPackage{}, marketregistry.ErrNotConfigured
	}
	resolved, err := a.client.Resolve(ctx, artifactID, marketregistry.ResolveRequest{VersionConstraint: versionConstraint})
	if err != nil {
		return marketplace.ResolvedPackage{}, err
	}
	return resolvedPackage(resolved), nil
}

func (a registryAdapter) Download(ctx context.Context, rawURL string) ([]byte, error) {
	if a.client == nil {
		return nil, marketregistry.ErrNotConfigured
	}
	return a.client.Download(ctx, rawURL)
}

func resolvedPackage(resolved marketregistry.ResolvedArtifact) marketplace.ResolvedPackage {
	return marketplace.ResolvedPackage{
		ArtifactID:     resolved.ArtifactID,
		Version:        resolved.Version,
		DownloadURL:    resolved.DownloadURL,
		ChecksumSHA256: resolved.ChecksumSHA256,
		SizeBytes:      resolved.SizeBytes,
		Compatibility:  resolved.Compatibility,
		Dependencies:   resolved.Dependencies,
		RiskLevel:      resolved.RiskLevel,
		TrustLevel:     resolved.TrustLevel,
		Permissions:    append([]string(nil), resolved.Permissions...),
		Signature:      resolved.Signature,
		Kind:           resolved.Kind,
		Name:           resolved.Name,
	}
}
