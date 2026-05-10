package marketregistry

import (
	"math"
	"regexp"
	"strings"
	"time"
)

var searchTokenPattern = regexp.MustCompile(`[\pL\pN_./:\-]+`)

func buildArtifactSearchDocument(artifact Artifact) map[string]string {
	return map[string]string{
		"artifact_id":      strings.TrimSpace(artifact.ID),
		"name":             strings.TrimSpace(artifact.Name),
		"summary":          strings.TrimSpace(artifact.Summary),
		"description_md":   strings.TrimSpace(artifact.DescriptionMD),
		"publisher":        strings.TrimSpace(artifact.Publisher),
		"kind":             strings.TrimSpace(string(artifact.Kind)),
		"tags_text":        strings.Join(trimmedStrings(artifact.Tags), " "),
		"hit_signals_text": strings.Join(trimmedStrings(artifact.HitSignals), " "),
		"use_case_text":    strings.TrimSpace(artifact.ManifestSummary["use_case"]),
		"search_text":      buildSearchText(artifact),
	}
}

func buildSearchText(artifact Artifact) string {
	parts := []string{
		strings.TrimSpace(artifact.ID),
		strings.TrimSpace(artifact.Name),
		strings.TrimSpace(artifact.Summary),
		strings.TrimSpace(artifact.DescriptionMD),
		strings.TrimSpace(artifact.Publisher),
		strings.TrimSpace(string(artifact.Kind)),
		strings.Join(trimmedStrings(artifact.Tags), " "),
		strings.Join(trimmedStrings(artifact.HitSignals), " "),
		strings.TrimSpace(artifact.ManifestSummary["use_case"]),
	}
	return strings.Join(compactStrings(parts...), " ")
}

func buildFTSQuery(query string) string {
	tokens := searchTokenPattern.FindAllString(strings.ToLower(strings.TrimSpace(query)), -1)
	if len(tokens) == 0 {
		cleaned := escapeFTSString(strings.TrimSpace(query))
		if cleaned == "" {
			return ""
		}
		return `"` + cleaned + `"`
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = escapeFTSString(token)
		if token == "" {
			continue
		}
		parts = append(parts, token+"*")
	}
	return strings.Join(parts, " AND ")
}

func escapeFTSString(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `"`, `""`)
	return value
}

func applySearchScores(candidate *searchCandidate, filter SearchFilter) {
	if candidate == nil {
		return
	}
	candidate.LexicalScore = lexicalScore(candidate.Artifact, filter.Query, candidate.lexicalRank)
	candidate.TagScore = tagScore(candidate.Artifact, filter.Tag, filter.Query)
	candidate.TrustScore = trustScore(candidate.TrustLevel)
	candidate.FreshnessScore = freshnessScore(candidate.UpdatedAt)
	candidate.RiskPenalty = riskPenalty(candidate.RiskLevel)
	candidate.FinalScore = finalScore(candidate.Artifact)
	candidate.Score = candidate.FinalScore
	candidate.MatchSignals = buildMatchSignals(candidate.Artifact)
}

func lexicalScore(artifact Artifact, query string, rank float64) float64 {
	query = normalizeSearchText(query)
	if query == "" {
		return 0
	}
	base := 0.0
	rank = math.Abs(rank)
	if rank > 0 {
		base = 1.0 / (1.0 + rank)
	}
	if rank == 0 {
		base = 0.55
	}
	idText := normalizeSearchText(artifact.ID)
	nameText := normalizeSearchText(artifact.Name)
	summaryText := normalizeSearchText(artifact.Summary)
	switch {
	case query == idText:
		base += 0.5
	case query == nameText:
		base += 0.4
	case strings.Contains(idText, query), strings.Contains(nameText, query):
		base += 0.2
	case strings.Contains(summaryText, query):
		base += 0.08
	}
	return clampScore(base)
}

func tagScore(artifact Artifact, tagFilter string, query string) float64 {
	if tagFilter != "" && containsFold(artifact.Tags, tagFilter) {
		return 1
	}
	queryTokens := tokenSet(query)
	if len(queryTokens) == 0 || len(artifact.Tags) == 0 {
		return 0
	}
	hits := 0
	for _, tag := range artifact.Tags {
		if _, ok := queryTokens[normalizeSearchText(tag)]; ok {
			hits++
		}
	}
	if hits == 0 {
		return 0
	}
	return clampScore(float64(hits) / float64(len(queryTokens)))
}

func trustScore(level string) float64 {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "verified":
		return 1
	case "trusted":
		return 0.8
	case "community":
		return 0.55
	default:
		return 0.35
	}
}

func freshnessScore(updatedAt string) float64 {
	if strings.TrimSpace(updatedAt) == "" {
		return 0
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(updatedAt))
	if err != nil {
		return 0
	}
	ageDays := time.Since(ts.UTC()).Hours() / 24
	switch {
	case ageDays <= 30:
		return 1
	case ageDays <= 90:
		return 0.8
	case ageDays <= 180:
		return 0.6
	case ageDays <= 365:
		return 0.35
	default:
		return 0.15
	}
}

func riskPenalty(level string) float64 {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "high":
		return 0.35
	case "medium":
		return 0.15
	default:
		return 0
	}
}

func finalScore(artifact Artifact) float64 {
	score := artifact.LexicalScore*0.70 +
		artifact.TagScore*0.10 +
		artifact.TrustScore*0.10 +
		artifact.FreshnessScore*0.10 -
		artifact.RiskPenalty
	return clampScore(score)
}

func clampScore(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Join(searchTokenPattern.FindAllString(value, -1), " ")
	return strings.TrimSpace(value)
}

func tokenSet(value string) map[string]struct{} {
	items := searchTokenPattern.FindAllString(strings.ToLower(strings.TrimSpace(value)), -1)
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = normalizeSearchText(item)
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func compactStrings(items ...string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func trimmedStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func buildMatchSignals(artifact Artifact) []string {
	signals := make([]string, 0, 4)
	if artifact.LexicalScore > 0 {
		signals = append(signals, "lexical")
	}
	if artifact.TagScore > 0 {
		signals = append(signals, "tag")
	}
	if artifact.TrustScore > 0 {
		signals = append(signals, "trust")
	}
	if artifact.FreshnessScore > 0 {
		signals = append(signals, "freshness")
	}
	return signals
}
