package markettools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	marketregistry "github.com/1024XEngineer/anyclaw/pkg/marketplace/registry"
)

type Options struct {
	Store            *marketplace.Store
	Registry         *marketregistry.Client
	AutoInstallSkill bool
	AuditLogger      tools.AuditLogger
	AfterInstall     func(ctx context.Context, receipt *marketplace.InstallReceipt) error
	AfterBind        func(ctx context.Context, binding *marketplace.Binding) error
}

func Register(registry *tools.Registry, opts Options) {
	if registry == nil || opts.Store == nil {
		return
	}
	registry.Register(&tools.Tool{
		Name:        "market_search_artifacts",
		Description: "Search installed and cloud marketplace artifacts for a missing capability, returning policy metadata and a recommended route.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":  map[string]string{"type": "string", "description": "Capability need or search query"},
				"kind":   map[string]string{"type": "string", "description": "Optional kind: agent, skill, or cli"},
				"source": map[string]string{"type": "string", "description": "Optional source: local, cloud, or all"},
				"limit":  map[string]string{"type": "number", "description": "Maximum results"},
			},
			"required": []string{"query"},
		},
		Category:    tools.ToolCategoryCustom,
		AccessLevel: tools.ToolAccessPublic,
		Visibility:  tools.ToolVisibilityMainAgentOnly,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return audit(opts, "market_search_artifacts", input, func(ctx context.Context, input map[string]any) (string, error) {
				return searchArtifacts(ctx, opts, input)
			})(ctx, input)
		},
	})
	registry.Register(&tools.Tool{
		Name:        "market_install_artifact",
		Description: "Install a cloud marketplace artifact under local policy. Ask decisions require explicit user_confirmed=true; high-risk permissions also require risk_acknowledged=true.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact_id":        map[string]string{"type": "string", "description": "Cloud artifact id"},
				"version_constraint": map[string]string{"type": "string", "description": "Optional exact version or version constraint"},
				"user_confirmed":     map[string]string{"type": "boolean", "description": "Set true only after user confirms the policy prompt"},
				"risk_acknowledged":  map[string]string{"type": "boolean", "description": "Set true only after user explicitly acknowledges high-risk permissions"},
			},
			"required": []string{"artifact_id"},
		},
		Category:         tools.ToolCategoryCustom,
		AccessLevel:      tools.ToolAccessPublic,
		Visibility:       tools.ToolVisibilityMainAgentOnly,
		RequiresApproval: true,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return audit(opts, "market_install_artifact", input, func(ctx context.Context, input map[string]any) (string, error) {
				if err := tools.RequestToolApproval(ctx, "market_install_artifact", input); err != nil {
					return "", err
				}
				return installArtifact(ctx, opts, input)
			})(ctx, input)
		},
	})
	registry.Register(&tools.Tool{
		Name:        "market_bind_artifact",
		Description: "Bind an installed marketplace artifact to main_agent, persistent_subagent, workspace, or runtime_global.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact_id": map[string]string{"type": "string", "description": "Installed artifact id"},
				"target_type": map[string]string{"type": "string", "description": "main_agent, persistent_subagent, workspace, or runtime_global"},
				"target_id":   map[string]string{"type": "string", "description": "Optional target id; runtime_global may omit it"},
			},
			"required": []string{"artifact_id", "target_type"},
		},
		Category:         tools.ToolCategoryCustom,
		AccessLevel:      tools.ToolAccessPublic,
		Visibility:       tools.ToolVisibilityMainAgentOnly,
		RequiresApproval: true,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return audit(opts, "market_bind_artifact", input, func(ctx context.Context, input map[string]any) (string, error) {
				if err := tools.RequestToolApproval(ctx, "market_bind_artifact", input); err != nil {
					return "", err
				}
				return bindArtifact(ctx, opts, input)
			})(ctx, input)
		},
	})
}

func searchArtifacts(ctx context.Context, opts Options, input map[string]any) (string, error) {
	query := stringValue(input["query"])
	kind := marketplace.NormalizeKind(stringValue(input["kind"]))
	source := marketplace.NormalizeSource(stringValue(input["source"]))
	limit := intValue(input["limit"], 5)
	local, err := localArtifacts(opts.Store, kind, limit)
	if err != nil {
		return "", err
	}
	var cloud []marketplace.Artifact
	var cloudErr string
	if source != marketplace.SourceLocal && opts.Registry != nil {
		result, err := opts.Registry.List(ctx, marketplace.Filter{Kind: kind, Query: query, Limit: limit})
		if err != nil {
			cloudErr = err.Error()
		} else {
			cloud = result.Items
		}
	}
	if source == marketplace.SourceCloud {
		local = nil
	}
	route := marketplace.RouteCapabilityNeed(query, local, cloud, limit)
	return marshalJSON(map[string]any{
		"query":       query,
		"kind":        kind,
		"source":      firstNonEmpty(string(source), "all"),
		"route":       route,
		"local_count": len(local),
		"cloud_count": len(cloud),
		"local":       marketplace.BuildCapabilityIndex(local),
		"cloud":       marketplace.BuildCapabilityIndex(cloud),
		"cloud_error": cloudErr,
	})
}

func installArtifact(ctx context.Context, opts Options, input map[string]any) (string, error) {
	if opts.Store == nil || opts.Registry == nil {
		return "", fmt.Errorf("marketplace install is not configured")
	}
	artifactID := strings.TrimSpace(stringValue(input["artifact_id"]))
	if artifactID == "" {
		return "", fmt.Errorf("artifact_id is required")
	}
	version := strings.TrimSpace(stringValue(input["version_constraint"]))
	userConfirmed := boolValue(input["user_confirmed"])
	riskAcknowledged := boolValue(input["risk_acknowledged"])
	resolved, err := opts.Registry.Resolve(ctx, artifactID, marketregistry.ResolveRequest{VersionConstraint: version})
	if err != nil {
		return "", err
	}
	resolvedPkg := resolvedPackage(resolved)
	decision := marketplace.NewDecisionPolicy(marketplace.PolicyConfig{AutoInstallSkill: opts.AutoInstallSkill}).DecideInstall(marketplace.InstallRequest{
		ArtifactID:        artifactID,
		VersionConstraint: version,
		InstalledBy:       "agent",
		UserConfirmed:     userConfirmed,
		RiskAcknowledged:  riskAcknowledged,
	}, resolvedPkg)
	if decision.Decision == marketplace.DecisionAsk && (decision.RequiresUserConfirmation || decision.RequiresRiskAcknowledgement) {
		_ = opts.Store.AppendAudit(marketplace.MarketAuditEvent{
			Type:       "market.agent_install.ask",
			ArtifactID: artifactID,
			Actor:      "agent",
			Decision:   string(decision.Decision),
			Reason:     decision.Reason,
			Detail: map[string]any{
				"version":     resolved.Version,
				"risk_level":  resolved.RiskLevel,
				"trust_level": resolved.TrustLevel,
				"permissions": resolved.Permissions,
			},
		})
		return marshalJSON(map[string]any{"status": "requires_confirmation", "decision": decision, "artifact": resolvedPkg})
	}
	uc := marketplace.NewInstallUseCaseWithPolicy(opts.Store, registryAdapter{client: opts.Registry}, marketplace.PolicyConfig{AutoInstallSkill: opts.AutoInstallSkill})
	job, reused, err := uc.Start(ctx, marketplace.InstallRequest{
		ArtifactID:        artifactID,
		VersionConstraint: version,
		InstalledBy:       "agent",
		UserConfirmed:     userConfirmed,
		RiskAcknowledged:  riskAcknowledged,
		IdempotencyKey:    "agent-" + artifactID + "-" + resolved.Version,
	})
	if err != nil {
		return "", err
	}
	if !reused {
		if err := uc.Execute(ctx, job.ID); err != nil {
			latest, _ := opts.Store.GetJob(job.ID)
			return marshalJSON(map[string]any{"status": "failed", "job": latest, "error": err.Error()})
		}
	}
	latest, _ := opts.Store.GetJob(job.ID)
	if latest != nil && latest.State == marketplace.JobSucceeded && strings.TrimSpace(latest.ReceiptID) != "" && opts.AfterInstall != nil {
		if receipt, receiptErr := opts.Store.GetReceipt(latest.ReceiptID); receiptErr == nil {
			if hookErr := opts.AfterInstall(ctx, receipt); hookErr != nil {
				return marshalJSON(map[string]any{"status": "installed", "job": latest, "reused": reused, "integration_error": hookErr.Error()})
			}
		}
	}
	return marshalJSON(map[string]any{"status": "installed", "job": latest, "reused": reused})
}

func bindArtifact(ctx context.Context, opts Options, input map[string]any) (string, error) {
	artifactID := strings.TrimSpace(stringValue(input["artifact_id"]))
	targetType := marketplace.NormalizeBindingTargetType(stringValue(input["target_type"]))
	if artifactID == "" || targetType == "" {
		return "", fmt.Errorf("artifact_id and target_type are required")
	}
	binding, err := opts.Store.CreateBinding(marketplace.BindingRequest{
		ArtifactID: artifactID,
		TargetType: targetType,
		TargetID:   strings.TrimSpace(stringValue(input["target_id"])),
	})
	if err != nil {
		return "", err
	}
	if opts.AfterBind != nil {
		if err := opts.AfterBind(ctx, binding); err != nil {
			return marshalJSON(map[string]any{"status": "bound", "binding": binding, "refresh_error": err.Error()})
		}
	}
	_ = opts.Store.AppendAudit(marketplace.MarketAuditEvent{
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
	return marshalJSON(map[string]any{"status": "bound", "binding": binding})
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

type registryAdapter struct {
	client *marketregistry.Client
}

func (a registryAdapter) Resolve(ctx context.Context, artifactID, versionConstraint string) (marketplace.ResolvedPackage, error) {
	resolved, err := a.client.Resolve(ctx, artifactID, marketregistry.ResolveRequest{VersionConstraint: versionConstraint})
	if err != nil {
		return marketplace.ResolvedPackage{}, err
	}
	return resolvedPackage(resolved), nil
}

func (a registryAdapter) Download(ctx context.Context, rawURL string) ([]byte, error) {
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

func audit(opts Options, toolName string, input map[string]any, next tools.ToolFunc) tools.ToolFunc {
	return func(ctx context.Context, _ map[string]any) (string, error) {
		output, err := next(ctx, input)
		if opts.AuditLogger != nil {
			opts.AuditLogger.LogTool(toolName, input, output, err)
		}
		return output, err
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || strings.EqualFold(v, "1") || strings.EqualFold(v, "yes")
	default:
		return false
	}
}

func intValue(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		i, err := json.Number(v).Int64()
		if err == nil {
			return int(i)
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func marshalJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
