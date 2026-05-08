package marketplace

import "strings"

func BuildCapabilityIndex(items []Artifact) []CapabilityIndexItem {
	out := make([]CapabilityIndexItem, 0, len(items))
	for _, item := range items {
		out = append(out, CapabilityIndexItem{
			ArtifactID:   item.ID,
			Kind:         item.Kind,
			Name:         firstNonEmpty(item.DisplayName, item.Name),
			Source:       item.Source,
			Status:       string(item.Status),
			Capabilities: artifactCapabilityTerms(item),
			Permissions:  append([]string(nil), item.Permissions...),
			RiskLevel:    item.RiskLevel,
			TrustLevel:   item.TrustLevel,
			Score:        item.Score,
		})
	}
	return out
}

func DetectCapabilityNeed(input string) string {
	normalized := strings.ToLower(strings.TrimSpace(input))
	switch {
	case containsAny(normalized, "release note", "changelog", "发布说明", "版本说明"):
		return "release notes"
	case containsAny(normalized, "code review", "review code", "pull request", "风险检查", "代码审查"):
		return "code review"
	case containsAny(normalized, "repo health", "repository health", "diagnose repo", "仓库健康", "诊断"):
		return "repo health"
	default:
		return strings.TrimSpace(input)
	}
}

func RouteCapabilityNeed(need string, installed []Artifact, cloud []Artifact, limit int) CapabilityRoute {
	need = DetectCapabilityNeed(need)
	if limit <= 0 {
		limit = 5
	}
	installedMatches := matchCapabilityIndex(BuildCapabilityIndex(installed), need, 1)
	if len(installedMatches) > 0 {
		match := installedMatches[0]
		return CapabilityRoute{
			Need:           need,
			InstalledMatch: &match,
			Action:         "use_installed",
			Reason:         "matching installed capability is already available",
		}
	}
	cloudMatches := matchCapabilityIndex(BuildCapabilityIndex(cloud), need, limit)
	if len(cloudMatches) > 0 {
		return CapabilityRoute{
			Need:         need,
			CloudMatches: cloudMatches,
			Action:       "install_from_market",
			Reason:       "cloud marketplace has matching capabilities",
		}
	}
	return CapabilityRoute{
		Need:   need,
		Action: "no_match",
		Reason: "no installed or cloud marketplace capability matched the need",
	}
}

func matchCapabilityIndex(items []CapabilityIndexItem, need string, limit int) []CapabilityIndexItem {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(need)))
	if len(terms) == 0 {
		return nil
	}
	var out []CapabilityIndexItem
	for _, item := range items {
		haystack := strings.ToLower(strings.Join(append([]string{item.ArtifactID, item.Name, string(item.Kind)}, item.Capabilities...), " "))
		matches := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			return out
		}
	}
	return out
}

func artifactCapabilityTerms(item Artifact) []string {
	return appendUniqueStrings(nil,
		append(append(append([]string{}, item.Capabilities...), item.Tags...), item.HitSignals...)...,
	)
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}
