package marketplace

import "testing"

func TestDecisionPolicyBlocksHighRiskAndQuarantined(t *testing.T) {
	policy := NewDecisionPolicy(PolicyConfig{AutoInstallSkill: true})
	decision := policy.DecideInstall(InstallRequest{UserConfirmed: true}, ResolvedPackage{
		ArtifactID:  "cloud.cli.danger",
		Kind:        ArtifactKindCLI,
		RiskLevel:   "high",
		TrustLevel:  "verified",
		Permissions: []string{"process.exec"},
	})
	if decision.Decision != DecisionBlock {
		t.Fatalf("decision = %s, want block", decision.Decision)
	}

	decision = policy.DecideInstall(InstallRequest{UserConfirmed: true}, ResolvedPackage{
		ArtifactID: "cloud.skill.quarantined",
		Kind:       ArtifactKindSkill,
		RiskLevel:  "low",
		TrustLevel: "quarantined",
	})
	if decision.Decision != DecisionBlock {
		t.Fatalf("quarantined decision = %s, want block", decision.Decision)
	}
}

func TestDecisionPolicyAutoInstallSkillRequiresConfigLowRiskAndVerified(t *testing.T) {
	resolved := ResolvedPackage{
		ArtifactID: "cloud.skill.release-notes",
		Kind:       ArtifactKindSkill,
		RiskLevel:  "low",
		TrustLevel: "verified",
	}
	decision := NewDecisionPolicy(PolicyConfig{AutoInstallSkill: true}).DecideInstall(InstallRequest{}, resolved)
	if decision.Decision != DecisionAuto {
		t.Fatalf("decision = %s, want auto", decision.Decision)
	}

	decision = NewDecisionPolicy(PolicyConfig{}).DecideInstall(InstallRequest{}, resolved)
	if decision.Decision != DecisionAsk || !decision.RequiresUserConfirmation {
		t.Fatalf("decision = %#v, want ask requiring confirmation", decision)
	}
}

func TestDecisionPolicyHighRiskPermissionRequiresExplicitAcknowledgement(t *testing.T) {
	resolved := ResolvedPackage{
		ArtifactID:  "cloud.skill.shell-helper",
		Kind:        ArtifactKindSkill,
		RiskLevel:   "low",
		TrustLevel:  "verified",
		Permissions: []string{"fs.read", "process.exec"},
	}
	policy := NewDecisionPolicy(PolicyConfig{AutoInstallSkill: true})

	decision := policy.DecideInstall(InstallRequest{UserConfirmed: true}, resolved)
	if decision.Decision != DecisionAsk || !decision.RequiresRiskAcknowledgement {
		t.Fatalf("decision = %#v, want ask requiring high-risk acknowledgement", decision)
	}
	if len(decision.HighRiskPermissions) != 1 || decision.HighRiskPermissions[0] != "process.exec" {
		t.Fatalf("high risk permissions = %#v", decision.HighRiskPermissions)
	}

	decision = policy.DecideInstall(InstallRequest{UserConfirmed: true, RiskAcknowledged: true}, resolved)
	if decision.Decision != DecisionAsk || decision.RequiresRiskAcknowledgement || decision.RequiresUserConfirmation {
		t.Fatalf("decision = %#v, want acknowledged ask", decision)
	}
}

func TestDecisionPolicyAskIsSatisfiedByUserConfirmation(t *testing.T) {
	decision := NewDecisionPolicy(PolicyConfig{}).DecideInstall(InstallRequest{UserConfirmed: true}, ResolvedPackage{
		ArtifactID: "cloud.agent.reviewer",
		Kind:       ArtifactKindAgent,
		RiskLevel:  "medium",
		TrustLevel: "verified",
	})
	if decision.Decision != DecisionAsk || decision.RequiresUserConfirmation {
		t.Fatalf("decision = %#v, want confirmed ask", decision)
	}
}
