package agentstore

import "testing"

func TestSummarizePackageBundleSkillOnly(t *testing.T) {
	pkg := AgentPackage{
		ID:          "skill-only",
		Name:        "skill-only",
		Description: "Standalone skill package",
	}

	bundle := summarizePackageBundle(pkg)
	if bundle.Mode != "skill" {
		t.Fatalf("expected skill mode, got %q", bundle.Mode)
	}
	if !bundle.IncludesSkill {
		t.Fatalf("expected skill in bundle: %#v", bundle)
	}
	if bundle.Skill != "skill-only" {
		t.Fatalf("expected skill-only skill, got %q", bundle.Skill)
	}
}

func TestSummarizePackageBundleGeneratesSkillWhenInstallSpecOmitsSkill(t *testing.T) {
	pkg := AgentPackage{
		ID:          "demo-package",
		Name:        "demo-package",
		Description: "Demo package with generated skill",
	}

	bundle := summarizePackageBundle(pkg)
	if bundle.Mode != "skill" {
		t.Fatalf("expected generated skill mode, got %q", bundle.Mode)
	}
	if !bundle.IncludesSkill {
		t.Fatalf("expected generated skill in bundle: %#v", bundle)
	}
	if bundle.Skill != "demo-package" {
		t.Fatalf("expected generated skill demo-package, got %q", bundle.Skill)
	}
}
