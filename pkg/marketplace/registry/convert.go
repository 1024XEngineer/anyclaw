package registry

import (
	"strconv"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
)

func convertArtifact(item remoteArtifact) marketplace.Artifact {
	metadata := map[string]string{}
	for key, value := range item.ManifestSummary {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			metadata[key] = value
		}
	}
	if item.ChecksumSHA256 != "" {
		metadata["checksum_sha256"] = item.ChecksumSHA256
	}
	if item.IconURL != "" {
		metadata["icon_url"] = item.IconURL
	}
	if item.UpdatedAt != "" {
		metadata["updated_at"] = item.UpdatedAt
	}
	if item.SizeBytes > 0 {
		metadata["size_bytes"] = strconv.FormatInt(item.SizeBytes, 10)
	}

	description := firstNonEmpty(item.DescriptionMD, item.Summary)
	version := firstNonEmpty(item.Version, item.LatestVersion)
	return marketplace.Artifact{
		ID:            item.ID,
		Kind:          item.Kind,
		Name:          item.Name,
		DisplayName:   item.Name,
		Description:   description,
		Version:       version,
		LatestVersion: item.LatestVersion,
		Source:        marketplace.SourceCloud,
		SourceID:      firstNonEmpty(item.Source, "registry"),
		Status:        marketplace.StatusAvailable,
		Installed:     false,
		Bound:         false,
		Active:        false,
		Enabled:       true,
		Owner:         item.Publisher,
		Category:      string(item.Kind),
		Tags:          append([]string(nil), item.Tags...),
		Permissions:   append([]string(nil), item.Permissions...),
		RiskLevel:     item.RiskLevel,
		TrustLevel:    item.TrustLevel,
		Verified:      strings.EqualFold(item.TrustLevel, "verified"),
		Compatibility: convertCompatibility(item.Compatibility),
		Dependencies:  convertDependencies(item.Dependencies),
		HitSignals:    append([]string(nil), item.HitSignals...),
		Score:         item.Score,
		TargetHints:   targetHintsForKind(item.Kind),
		Capabilities:  appendUnique(nil, append(append(item.Tags, item.HitSignals...), string(item.Kind))...),
		Metadata:      metadata,
	}
}

func convertVersion(item remoteVersion) marketplace.ArtifactVersion {
	return marketplace.ArtifactVersion{
		Version:         item.Version,
		ReleasedAt:      item.ReleasedAt,
		ChangelogMD:     item.ChangelogMD,
		Compatibility:   convertCompatibility(item.Compatibility),
		PermissionsDiff: append([]string(nil), item.PermissionsDiff...),
		SizeBytes:       item.SizeBytes,
		Deprecated:      item.Deprecated,
	}
}

func convertCompatibility(item remoteCompatibility) marketplace.Compatibility {
	return marketplace.Compatibility{
		AnyClawMin: item.AnyClawMin,
		OS:         append([]string(nil), item.OS...),
		Arch:       append([]string(nil), item.Arch...),
	}
}

func convertDependencies(items []remoteDependency) []marketplace.ArtifactDependency {
	out := make([]marketplace.ArtifactDependency, 0, len(items))
	for _, item := range items {
		out = append(out, marketplace.ArtifactDependency{
			ID:           item.ID,
			VersionRange: item.VersionRange,
		})
	}
	return out
}

func targetHintsForKind(kind marketplace.ArtifactKind) []string {
	switch kind {
	case marketplace.ArtifactKindAgent:
		return []string{"main_agent", "persistent_subagent", "workspace"}
	case marketplace.ArtifactKindSkill:
		return []string{"main_agent", "persistent_subagent", "workspace"}
	case marketplace.ArtifactKindCLI:
		return []string{"workspace", "runtime_global"}
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUnique(base []string, values ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out)+len(values))
	filtered := make([]string, 0, len(out)+len(values))
	for _, value := range append(out, values...) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, trimmed)
	}
	return filtered
}
