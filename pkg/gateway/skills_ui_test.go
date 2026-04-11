package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestHandleSkillsPersistsTogglesForMainAgent(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workflows")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")

	if err := writeTestSkill(cfg.Skills.Dir, "toggle-alpha", "alpha"); err != nil {
		t.Fatalf("write alpha skill: %v", err)
	}
	if err := writeTestSkill(cfg.Skills.Dir, "toggle-beta", "beta"); err != nil {
		t.Fatalf("write beta skill: %v", err)
	}

	configPath := filepath.Join(tempDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	app := &appRuntime.App{
		ConfigPath: configPath,
		Config:     cfg,
		WorkDir:    cfg.Agent.WorkDir,
		WorkingDir: cfg.Agent.WorkingDir,
	}
	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := &Server{
		app:   app,
		store: store,
	}

	postReq := httptest.NewRequest(http.MethodPost, "/skills", strings.NewReader(`{"name":"toggle-beta","enabled":false}`))
	postReq = postReq.WithContext(context.WithValue(postReq.Context(), authUserKey, &AuthUser{
		Name:        "operator",
		Permissions: []string{"config.write"},
	}))
	postRec := httptest.NewRecorder()

	server.handleSkills(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	updatedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	alphaRef, alphaOK := findSkillRef(updatedCfg.Agent.Skills, "toggle-alpha")
	if !alphaOK {
		t.Fatalf("expected toggle-alpha to be materialized in config")
	}
	if !alphaRef.Enabled {
		t.Fatalf("expected toggle-alpha to stay enabled")
	}

	betaRef, betaOK := findSkillRef(updatedCfg.Agent.Skills, "toggle-beta")
	if !betaOK {
		t.Fatalf("expected toggle-beta to be saved in config")
	}
	if betaRef.Enabled {
		t.Fatalf("expected toggle-beta to be disabled after toggle")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/skills", nil)
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"skills.read"},
	}))
	getRec := httptest.NewRecorder()

	server.handleSkills(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected GET 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload []skillView
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	alphaView, alphaViewOK := findSkillView(payload, "toggle-alpha")
	if !alphaViewOK {
		t.Fatalf("expected toggle-alpha in response")
	}
	if !alphaView.Enabled || !alphaView.Loaded {
		t.Fatalf("expected toggle-alpha to remain enabled and loaded: %#v", alphaView)
	}

	betaView, betaViewOK := findSkillView(payload, "toggle-beta")
	if !betaViewOK {
		t.Fatalf("expected toggle-beta in response")
	}
	if betaView.Enabled || betaView.Loaded {
		t.Fatalf("expected toggle-beta to be disabled and unloaded: %#v", betaView)
	}
}

func writeTestSkill(skillsDir string, name string, description string) error {
	skillDir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	body := `{"name":"` + name + `","description":"` + description + `","version":"1.0.0","commands":[],"prompts":{}}`
	return os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(body), 0o644)
}

func findSkillRef(items []config.AgentSkillRef, name string) (config.AgentSkillRef, bool) {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(name)) {
			return item, true
		}
	}
	return config.AgentSkillRef{}, false
}

func findSkillView(items []skillView, name string) (skillView, bool) {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(name)) {
			return item, true
		}
	}
	return skillView{}, false
}
