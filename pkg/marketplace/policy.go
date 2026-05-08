package marketplace

import (
	"fmt"
	"runtime"
	"strings"
)

type PolicyConfig struct {
	AutoInstallSkill bool
}

type DecisionPolicy struct {
	cfg PolicyConfig
}

func NewDecisionPolicy(cfg PolicyConfig) DecisionPolicy {
	return DecisionPolicy{cfg: cfg}
}

func (p DecisionPolicy) DecideInstall(req InstallRequest, resolved ResolvedPackage) PolicyDecision {
	risk := normalizedRisk(resolved.RiskLevel)
	trust := normalizedTrust(resolved.TrustLevel)
	highRiskPermissions := highRiskPermissions(resolved.Permissions)

	reasons := policyBlockReasons(resolved)
	if len(reasons) > 0 {
		return PolicyDecision{
			Decision:            DecisionBlock,
			Reason:              strings.Join(reasons, "; "),
			Reasons:             reasons,
			RiskLevel:           risk,
			TrustLevel:          trust,
			Permissions:         append([]string(nil), resolved.Permissions...),
			HighRiskPermissions: highRiskPermissions,
		}
	}

	if p.cfg.AutoInstallSkill && resolved.Kind == ArtifactKindSkill && risk == "low" && trust == "verified" && len(highRiskPermissions) == 0 {
		return PolicyDecision{
			Decision:    DecisionAuto,
			Reason:      "low-risk verified skill auto install is enabled",
			Reasons:     []string{"low-risk verified skill auto install is enabled"},
			RiskLevel:   risk,
			TrustLevel:  trust,
			Permissions: append([]string(nil), resolved.Permissions...),
		}
	}

	reasons = installRiskReasons(resolved, highRiskPermissions)
	reason := strings.Join(reasons, "; ")
	if reason == "" {
		reason = fmt.Sprintf("%s artifacts require user confirmation", resolved.Kind)
		reasons = []string{reason}
	}
	if req.UserConfirmed {
		reasons = append(reasons, "user confirmed marketplace install")
		reason = strings.Join(reasons, "; ")
	}
	requiresRiskAcknowledgement := len(highRiskPermissions) > 0 && !req.RiskAcknowledged
	if req.RiskAcknowledged && len(highRiskPermissions) > 0 {
		reasons = append(reasons, "user acknowledged high-risk permissions")
		reason = strings.Join(reasons, "; ")
	}
	return PolicyDecision{
		Decision:                    DecisionAsk,
		Reason:                      reason,
		Reasons:                     appendUniqueStrings(nil, reasons...),
		RequiresUserConfirmation:    !req.UserConfirmed,
		RequiresRiskAcknowledgement: requiresRiskAcknowledgement,
		RiskLevel:                   risk,
		TrustLevel:                  trust,
		Permissions:                 append([]string(nil), resolved.Permissions...),
		HighRiskPermissions:         highRiskPermissions,
	}
}

func policyBlockReasons(resolved ResolvedPackage) []string {
	var reasons []string
	risk := normalizedRisk(resolved.RiskLevel)
	if risk == "high" {
		reasons = append(reasons, "high-risk artifacts are blocked")
	}
	if normalizedTrust(resolved.TrustLevel) == "quarantined" {
		reasons = append(reasons, "quarantined artifacts are blocked")
	}
	for _, permission := range resolved.Permissions {
		if highRiskPermission(permission) && risk == "high" {
			reasons = append(reasons, "high-risk permission "+strings.TrimSpace(permission)+" is blocked")
		}
	}
	if !compatibleWithCurrentRuntime(resolved.Compatibility) {
		reasons = append(reasons, "artifact is incompatible with this OS or architecture")
	}
	return appendUniqueStrings(nil, reasons...)
}

func installRiskReasons(resolved ResolvedPackage, highRiskPermissions []string) []string {
	reasons := []string{fmt.Sprintf("%s artifacts require user confirmation", resolved.Kind)}
	if risk := normalizedRisk(resolved.RiskLevel); risk != "" {
		reasons = append(reasons, "risk level: "+risk)
	}
	if trust := normalizedTrust(resolved.TrustLevel); trust != "" {
		reasons = append(reasons, "trust level: "+trust)
	}
	if len(highRiskPermissions) > 0 {
		reasons = append(reasons, "high-risk permissions require explicit acknowledgement: "+strings.Join(highRiskPermissions, ", "))
	}
	return appendUniqueStrings(nil, reasons...)
}

func highRiskPermissions(values []string) []string {
	var permissions []string
	for _, permission := range values {
		if highRiskPermission(permission) {
			permissions = append(permissions, strings.TrimSpace(permission))
		}
	}
	return appendUniqueStrings(nil, permissions...)
}

func normalizedRisk(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "medium"
	}
	return value
}

func normalizedTrust(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func highRiskPermission(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "process.exec", "process.kill", "desktop.control", "browser.control", "network.any", "secrets.read", "fs.delete":
		return true
	default:
		return false
	}
}

func compatibleWithCurrentRuntime(compat Compatibility) bool {
	if len(compat.OS) > 0 && !containsNormalized(compat.OS, runtime.GOOS) {
		return false
	}
	if len(compat.Arch) > 0 && !containsNormalized(compat.Arch, runtime.GOARCH) {
		return false
	}
	return true
}

func containsNormalized(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}
	return false
}

func appendUniqueStrings(base []string, values ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out)+len(values))
	for _, value := range out {
		seen[strings.ToLower(strings.TrimSpace(value))] = true
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	return out
}
