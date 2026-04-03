package agentstore

import "testing"

func TestSummarizePackageBundleWithAppGeneratesBundledSkill(t *testing.T) {
	pkg := AgentPackage{
		ID:          "demo-package",
		Name:        "demo-package",
		Description: "Demo package with generated skill",
		Install: &InstallSpec{
			App: &AppInstallSpec{Plugin: "demo-app"},
		},
	}

	bundle := summarizePackageBundle(pkg)
	if bundle.Mode != "bundle" {
		t.Fatalf("expected bundle mode, got %q", bundle.Mode)
	}
	if !bundle.IncludesApp || !bundle.IncludesSkill {
		t.Fatalf("expected app and skill to be included: %#v", bundle)
	}
	if bundle.App != "demo-app" {
		t.Fatalf("expected app demo-app, got %q", bundle.App)
	}
	if bundle.Skill != "demo-package" {
		t.Fatalf("expected generated skill demo-package, got %q", bundle.Skill)
	}
}

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
	if bundle.IncludesApp {
		t.Fatalf("expected no app in bundle: %#v", bundle)
	}
	if !bundle.IncludesSkill {
		t.Fatalf("expected skill in bundle: %#v", bundle)
	}
	if bundle.Skill != "skill-only" {
		t.Fatalf("expected skill-only skill, got %q", bundle.Skill)
	}
}
