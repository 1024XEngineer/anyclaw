package skills

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestBuildViews(t *testing.T) {
	if got := BuildViews(nil, nil); got != nil {
		t.Fatalf("expected nil manager views to be nil, got %#v", got)
	}

	manager := NewSkillsManager("")
	manager.skills["Runner"] = &Skill{
		Name:           "Runner",
		Description:    "Runs",
		Version:        "2.0.0",
		Permissions:    []string{"tools:exec"},
		Entrypoint:     "run.ps1",
		Registry:       "local",
		Source:         "local",
		InstallCommand: "manual",
	}
	manager.skills["Planner"] = &Skill{
		Name:           "Planner",
		Description:    "Plans",
		Version:        "1.0.0",
		Permissions:    []string{"files:read"},
		Entrypoint:     "builtin://planner",
		Registry:       "builtin",
		Source:         "builtin",
		InstallCommand: "anyclaw skill install planner",
	}
	manager.skills[" "] = &Skill{Name: " "}

	views := BuildViews(manager, []config.AgentSkillRef{{Name: "planner", Enabled: false}, {Name: "runner", Enabled: true}, {Name: "runner", Enabled: false}})
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}
	if !views[0].Loaded || views[0].Name != "Runner" {
		t.Fatalf("expected loaded runner first, got %#v", views[0])
	}
	if views[1].Enabled || views[1].Name != "Planner" {
		t.Fatalf("expected disabled planner second, got %#v", views[1])
	}

	defaultViews := BuildViews(manager, nil)
	if len(defaultViews) != 2 || !defaultViews[0].Enabled || !defaultViews[1].Enabled {
		t.Fatalf("expected default views to be enabled, got %#v", defaultViews)
	}
}

func TestMaterializeRefsAndNormalizeKey(t *testing.T) {
	installed := []*Skill{
		nil,
		{Name: "Runner", Version: "2.0.0", Permissions: []string{"tools:exec"}},
		{Name: "planner", Version: "1.0.0", Permissions: []string{"files:read"}},
		{Name: "Runner", Version: "ignored"},
		{Name: "   "},
	}
	existing := []config.AgentSkillRef{
		{Name: "runner", Enabled: false},
		{Name: "ghost", Enabled: true, Version: "9.9.9"},
		{Name: "ghost", Enabled: false},
	}

	refs := MaterializeRefs(installed, existing)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %#v", refs)
	}
	if refs[0].Name != "planner" || refs[0].Enabled || refs[0].Version != "1.0.0" {
		t.Fatalf("unexpected planner ref: %#v", refs[0])
	}
	if refs[1].Name != "Runner" || refs[1].Enabled || refs[1].Version != "2.0.0" || len(refs[1].Permissions) != 1 {
		t.Fatalf("unexpected runner ref: %#v", refs[1])
	}
	if refs[2].Name != "ghost" || refs[2].Version != "9.9.9" {
		t.Fatalf("unexpected ghost ref: %#v", refs[2])
	}

	if got := NormalizeKey("  Runner "); got != "runner" {
		t.Fatalf("unexpected normalized key %q", got)
	}
}
