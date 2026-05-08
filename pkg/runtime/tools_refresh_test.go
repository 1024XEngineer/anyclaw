package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

func TestRefreshToolRegistrySynchronizesMainAgentTools(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.PermissionLevel = "read-only"
	cfg.Agent.WorkingDir = tempDir
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, "audit.jsonl")

	mem := memory.NewFileMemory(tempDir)
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	oldRegistry := tools.NewRegistry()
	oldRegistry.RegisterTool("stale_tool", "stale test tool", nil, func(ctx context.Context, input map[string]any) (string, error) {
		return "stale", nil
	})
	tools.RegisterBuiltins(oldRegistry, tools.BuiltinOptions{
		WorkingDir:      tempDir,
		PermissionLevel: "full",
	})

	ag := agent.New(agent.Config{
		Name:             "main",
		Memory:           mem,
		Skills:           skills.NewSkillsManager(cfg.Skills.Dir),
		Tools:            oldRegistry,
		MaxContextTokens: 4096,
		LLM: &refreshToolLLM{
			toolName: "stale_tool",
		},
	})
	rt := &MainRuntime{
		ConfigPath: filepath.Join(tempDir, "anyclaw.json"),
		Config:     cfg,
		Agent:      ag,
		Memory:     mem,
		Skills:     skills.NewSkillsManager(cfg.Skills.Dir),
		Tools:      oldRegistry,
		WorkDir:    tempDir,
		WorkingDir: tempDir,
	}

	if !hasAgentTool(ag, "stale_tool") {
		t.Fatal("expected stale tool to be visible before refresh")
	}
	result, err := ag.Run(context.Background(), "call stale_tool now")
	if err != nil {
		t.Fatalf("expected old registry to execute stale tool before refresh: %v", err)
	}
	if result != "done" {
		t.Fatalf("expected final LLM response after stale tool call, got %q", result)
	}

	if err := rt.RefreshToolRegistry(); err != nil {
		t.Fatalf("RefreshToolRegistry: %v", err)
	}
	if hasAgentTool(ag, "stale_tool") {
		t.Fatal("expected Agent tool registry to be replaced after refresh")
	}
	if _, ok := rt.Tools.Get("stale_tool"); ok {
		t.Fatal("expected runtime tool registry to be replaced after refresh")
	}

	_, err = ag.Run(context.Background(), "call stale_tool now")
	if err != nil {
		t.Fatalf("expected Agent to continue after stale tool execution error, got %v", err)
	}
	activities := ag.GetLastToolActivities()
	if len(activities) == 0 || activities[0].ToolName != "stale_tool" || !strings.Contains(activities[0].Error, "tool not found: stale_tool") {
		t.Fatalf("expected Agent to execute against refreshed registry and record stale tool error, got %#v", activities)
	}
	_, err = rt.CallTool(context.Background(), "write_file", map[string]any{
		"path":    filepath.Join(tempDir, "after.txt"),
		"content": "after",
	})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected refreshed runtime registry to enforce read-only, got %v", err)
	}
}

func TestMarketInstallToolIntegratesSkillAndRefreshesRuntime(t *testing.T) {
	tempDir := t.TempDir()
	archive := runtimeMarketArchive(t, "cloud.skill.release-notes", marketplace.ArtifactKindSkill, "1.0.0")
	server := runtimeMarketRegistryServer(t, "cloud.skill.release-notes", "skill", archive)
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = tempDir
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, "audit.jsonl")
	cfg.Marketplace.RegistryEndpoint = server.URL
	cfg.Marketplace.AutoInstallSkill = true
	cfg.Marketplace.DisableRemote = false
	cfg.Marketplace.ProtocolVersion = "v1"
	cfg.Marketplace.RequestTimeoutSeconds = 5
	cfg.Marketplace.DownloadTimeoutSeconds = 5

	rt, err := NewMainRuntimeFromConfig(filepath.Join(tempDir, "anyclaw.json"), cfg)
	if err != nil {
		t.Fatalf("NewMainRuntimeFromConfig: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	if hasAgentTool(rt.Agent, "skill_cloud.skill.release-notes") {
		t.Fatal("did not expect market skill tool before install")
	}

	out, err := rt.CallTool(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent}), "market_install_artifact", map[string]any{
		"artifact_id": "cloud.skill.release-notes",
	})
	if err != nil {
		t.Fatalf("market_install_artifact: %v", err)
	}
	if !strings.Contains(out, `"status": "installed"`) {
		t.Fatalf("install output = %s, want installed", out)
	}
	if !hasAgentTool(rt.Agent, "skill_cloud.skill.release-notes") {
		t.Fatalf("expected installed skill tool after integration refresh, tools=%#v", rt.Agent.ListTools())
	}
	if !hasAgentTool(rt.Agent, "market_search_artifacts") {
		t.Fatal("expected marketplace tools to remain registered after refresh")
	}
}

func TestRefreshToolRegistryUsesRuntimeWorkDirMarketplaceStore(t *testing.T) {
	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, ".anyclaw")
	workingDir := filepath.Join(tempDir, "workspace")
	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = workDir
	cfg.Agent.WorkingDir = workingDir
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")

	store := marketplace.NewStore(workDir)
	if err := store.SaveReceipt(&marketplace.InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: filepath.Join(workDir, "installed"),
		InstalledBy:   "user",
		InstalledAt:   "2026-05-07T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	rt := &MainRuntime{
		ConfigPath: filepath.Join(tempDir, "anyclaw.json"),
		Config:     cfg,
		Skills:     skills.NewSkillsManager(cfg.Skills.Dir),
		WorkDir:    workDir,
		WorkingDir: workingDir,
	}
	if err := rt.RefreshToolRegistry(); err != nil {
		t.Fatalf("RefreshToolRegistry: %v", err)
	}
	out, err := rt.CallTool(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent}), "market_search_artifacts", map[string]any{
		"query":  "release notes",
		"kind":   "skill",
		"source": "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cloud.skill.release-notes") {
		t.Fatalf("expected refreshed marketplace tool to read WorkDir store, got %s", out)
	}
}

type refreshToolLLM struct {
	toolName string
	calls    int
}

func (l *refreshToolLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	l.calls++
	if l.calls%2 == 1 {
		return &llm.Response{
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-1",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      l.toolName,
					Arguments: `{}`,
				},
			}},
		}, nil
	}
	return &llm.Response{Content: "done"}, nil
}

func (l *refreshToolLLM) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error {
	resp, err := l.Chat(ctx, messages, tools)
	if err != nil {
		return err
	}
	if onChunk != nil {
		onChunk(resp.Content)
	}
	return nil
}

func (l *refreshToolLLM) Name() string {
	return "refresh-tool-llm"
}

func hasAgentTool(ag *agent.Agent, name string) bool {
	for _, item := range ag.ListTools() {
		if item.Name == name {
			return true
		}
	}
	return false
}

func runtimeMarketRegistryServer(t *testing.T, id, kind string, archive []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/resolve"):
			writeRuntimeMarketJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     id,
				"version":         "1.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/" + id + "/1.0.0",
				"checksum_sha256": runtimeMarketSHA256(archive),
				"size_bytes":      len(archive),
				"risk_level":      "low",
				"trust_level":     "verified",
				"permissions":     []string{"fs.read"},
				"kind":            kind,
				"name":            id,
			}})
		case strings.Contains(r.URL.Path, "/v1/download/"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
}

func runtimeMarketArchive(t *testing.T, id string, kind marketplace.ArtifactKind, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	writeZipJSONRuntimeMarket(t, writer, "anyclaw.artifact.json", map[string]any{
		"id":          id,
		"kind":        string(kind),
		"name":        id,
		"summary":     "Release notes helper",
		"description": "Draft release notes.",
		"version":     version,
	})
	writeZipTextRuntimeMarket(t, writer, "skill/SKILL.md", "# Release Notes\n\nDraft release notes.\n")
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func writeZipJSONRuntimeMarket(t *testing.T, writer *zip.Writer, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal zip json: %v", err)
	}
	writeZipTextRuntimeMarket(t, writer, name, string(data))
}

func writeZipTextRuntimeMarket(t *testing.T, writer *zip.Writer, name string, value string) {
	t.Helper()
	file, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create zip entry %s: %v", name, err)
	}
	if _, err := file.Write([]byte(value)); err != nil {
		t.Fatalf("write zip entry %s: %v", name, err)
	}
}

func writeRuntimeMarketJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func runtimeMarketSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
