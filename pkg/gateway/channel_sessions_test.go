package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type groupTestLLM struct {
	reply func(messages []llm.Message) string
}

func (g *groupTestLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: g.reply(messages)}, nil
}

func (g *groupTestLLM) Name() string {
	return "group-test"
}

func TestRunSessionMessageGroupChannelSharesAgentReplies(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	workspacePath := t.TempDir()
	if err := store.UpsertWorkspace(&Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: workspacePath}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	makeApp := func(name string, client llm.Client) *appRuntime.App {
		mem := memory.NewFileMemory(t.TempDir())
		if err := mem.Init(); err != nil {
			t.Fatalf("memory init: %v", err)
		}
		registry := tools.NewRegistry()
		ag := agent.New(agent.Config{
			Name:        name,
			Description: name,
			LLM:         client,
			Memory:      mem,
			Skills:      skills.NewSkillsManager(""),
			Tools:       registry,
			WorkDir:     t.TempDir(),
		})
		return &appRuntime.App{
			Config: &config.Config{
				Agent: config.AgentConfig{Name: name},
			},
			Agent:      ag,
			WorkingDir: workspacePath,
			WorkDir:    t.TempDir(),
		}
	}

	appOne := makeApp("AgentOne", &groupTestLLM{
		reply: func(messages []llm.Message) string {
			return "AgentOne: 已完成初步分析"
		},
	})
	appTwo := makeApp("AgentTwo", &groupTestLLM{
		reply: func(messages []llm.Message) string {
			input := messages[len(messages)-1].Content
			if !strings.Contains(input, "AgentOne: 已完成初步分析") {
				return "AgentTwo: 未看到队友结论"
			}
			return "AgentTwo: 已基于 AgentOne 的结论继续补充"
		},
	})

	pool := NewRuntimePool("ignored", store, 4, time.Hour)
	now := time.Now().UTC()
	pool.runtimes[runtimeKey("AgentOne", "org-1", "project-1", "workspace-1")] = &runtimeEntry{app: appOne, createdAt: now, lastUsedAt: now}
	pool.runtimes[runtimeKey("AgentTwo", "org-1", "project-1", "workspace-1")] = &runtimeEntry{app: appTwo, createdAt: now, lastUsedAt: now}

	sessionAgent := appOne.Agent
	sessions := NewSessionManager(store, sessionAgent)
	session, err := sessions.CreateWithOptions(SessionCreateOptions{
		Title:        "协作频道",
		AgentName:    "AgentOne",
		Participants: []string{"AgentOne", "AgentTwo"},
		Org:          "org-1",
		Project:      "project-1",
		Workspace:    "workspace-1",
		IsGroup:      true,
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	server := &Server{
		store:       store,
		sessions:    sessions,
		bus:         NewBus(),
		runtimePool: pool,
	}

	response, updatedSession, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "请一起拆解这个任务")
	if err != nil {
		t.Fatalf("runSessionMessage: %v", err)
	}
	if !strings.Contains(response, "AgentOne: 已完成初步分析") {
		t.Fatalf("expected AgentOne response in summary, got %q", response)
	}
	if !strings.Contains(response, "AgentTwo: 已基于 AgentOne 的结论继续补充") {
		t.Fatalf("expected AgentTwo response in summary, got %q", response)
	}
	if updatedSession == nil {
		t.Fatal("expected updated session")
	}
	if len(updatedSession.Messages) != 3 {
		t.Fatalf("expected 3 channel messages, got %d", len(updatedSession.Messages))
	}
	if updatedSession.Messages[1].Agent != "AgentOne" {
		t.Fatalf("expected first assistant message from AgentOne, got %+v", updatedSession.Messages[1])
	}
	if updatedSession.Messages[2].Agent != "AgentTwo" {
		t.Fatalf("expected second assistant message from AgentTwo, got %+v", updatedSession.Messages[2])
	}
	if !strings.Contains(updatedSession.Messages[2].Content, "继续补充") {
		t.Fatalf("expected AgentTwo to see AgentOne reply, got %q", updatedSession.Messages[2].Content)
	}
}
