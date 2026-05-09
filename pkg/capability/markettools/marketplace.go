package markettools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	marketbridge "github.com/1024XEngineer/anyclaw/pkg/marketplace/bridge"
)

type Options struct {
	Bridge      marketbridge.Bridge
	AuditLogger tools.AuditLogger
}

type retrievalMetaPayload struct {
	SearchMode           string         `json:"search_mode,omitempty"`
	VectorApplied        *bool          `json:"vector_applied,omitempty"`
	VectorFallbackReason string         `json:"vector_fallback_reason,omitempty"`
	CandidateCounts      map[string]int `json:"candidate_counts,omitempty"`
}

func Register(registry *tools.Registry, opts Options) {
	if registry == nil || opts.Bridge == nil {
		return
	}
	registry.Register(&tools.Tool{
		Name:        "market_search_artifacts",
		Description: "Search installed and cloud marketplace artifacts for a missing capability, returning policy metadata and a recommended route.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]string{"type": "string", "description": "Capability need or search query"},
				"kind":        map[string]string{"type": "string", "description": "Optional kind: agent, skill, or cli"},
				"source":      map[string]string{"type": "string", "description": "Optional source: local, cloud, or all"},
				"search_mode": map[string]string{"type": "string", "description": "Optional search mode: auto, lexical, or hybrid"},
				"limit":       map[string]string{"type": "number", "description": "Maximum results"},
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
	searchMode := marketplace.NormalizeSearchMode(stringValue(input["search_mode"]))
	limit := intValue(input["limit"], 5)
	result, err := opts.Bridge.Search(ctx, marketbridge.SearchRequest{
		Query:      query,
		Kind:       kind,
		Source:     source,
		SearchMode: searchMode,
		Limit:      limit,
	})
	if err != nil {
		return "", err
	}
	route := marketplace.RouteCapabilityNeed(query, result.Local, result.Cloud, limit)
	return marshalJSON(map[string]any{
		"query":           query,
		"kind":            kind,
		"source":          firstNonEmpty(string(source), "all"),
		"route":           route,
		"local_count":     len(result.Local),
		"cloud_count":     len(result.Cloud),
		"local":           marketplace.BuildCapabilityIndex(result.Local),
		"cloud":           marketplace.BuildCapabilityIndex(result.Cloud),
		"cloud_artifacts": result.Cloud,
		"cloud_error":     result.CloudErr,
		"retrieval_meta":  buildRetrievalMetaPayload(result.RetrievalMeta),
	})
}

func installArtifact(ctx context.Context, opts Options, input map[string]any) (string, error) {
	if opts.Bridge == nil {
		return "", fmt.Errorf("marketplace install is not configured")
	}
	artifactID := strings.TrimSpace(stringValue(input["artifact_id"]))
	if artifactID == "" {
		return "", fmt.Errorf("artifact_id is required")
	}
	version := strings.TrimSpace(stringValue(input["version_constraint"]))
	userConfirmed := boolValue(input["user_confirmed"])
	riskAcknowledged := boolValue(input["risk_acknowledged"])
	req := marketplace.InstallRequest{
		ArtifactID:        artifactID,
		VersionConstraint: version,
		InstalledBy:       "agent",
		UserConfirmed:     userConfirmed,
		RiskAcknowledged:  riskAcknowledged,
	}
	plan, err := opts.Bridge.PlanInstall(ctx, req)
	if err != nil {
		return "", err
	}
	if plan.Decision.Decision == marketplace.DecisionAsk && (plan.Decision.RequiresUserConfirmation || plan.Decision.RequiresRiskAcknowledgement) {
		return marshalJSON(map[string]any{"status": "requires_confirmation", "decision": plan.Decision, "artifact": plan.Artifact})
	}
	req.IdempotencyKey = "agent-" + artifactID + "-" + plan.Artifact.Version
	result, err := opts.Bridge.Install(ctx, req)
	if err != nil {
		return marshalJSON(map[string]any{"status": "failed", "job": result.Job, "error": err.Error()})
	}
	return marshalJSON(map[string]any{"status": "installed", "job": result.Job, "reused": result.Reused})
}

func bindArtifact(ctx context.Context, opts Options, input map[string]any) (string, error) {
	artifactID := strings.TrimSpace(stringValue(input["artifact_id"]))
	targetType := marketplace.NormalizeBindingTargetType(stringValue(input["target_type"]))
	if artifactID == "" || targetType == "" {
		return "", fmt.Errorf("artifact_id and target_type are required")
	}
	if opts.Bridge == nil {
		return "", fmt.Errorf("marketplace bridge is not configured")
	}
	binding, err := opts.Bridge.Bind(ctx, marketplace.BindingRequest{
		ArtifactID: artifactID,
		TargetType: targetType,
		TargetID:   strings.TrimSpace(stringValue(input["target_id"])),
	})
	if err != nil {
		return "", err
	}
	return marshalJSON(map[string]any{"status": "bound", "binding": binding})
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

func buildRetrievalMetaPayload(meta *marketplace.RetrievalMeta) *retrievalMetaPayload {
	if meta == nil {
		return nil
	}
	payload := &retrievalMetaPayload{
		SearchMode:           string(meta.SearchMode),
		VectorApplied:        meta.VectorApplied,
		VectorFallbackReason: meta.VectorFallbackReason,
	}
	if meta.CandidateCounts != nil {
		payload.CandidateCounts = map[string]int{
			"lexical": meta.CandidateCounts.Lexical,
			"vector":  meta.CandidateCounts.Vector,
			"merged":  meta.CandidateCounts.Merged,
		}
	}
	return payload
}
