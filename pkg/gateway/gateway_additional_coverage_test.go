package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	chatintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake/chat"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type fakeChatManager struct{}

func (fakeChatManager) Chat(context.Context, chatintake.ChatRequest) (*chatintake.ChatResponse, error) {
	return &chatintake.ChatResponse{SessionID: "s1", AgentName: "agent", Message: chatintake.Message{Role: "assistant", Content: "ok"}}, nil
}

func (fakeChatManager) GetSession(string) (*chatintake.Session, error) { return nil, nil }
func (fakeChatManager) ListSessions() []chatintake.Session             { return []chatintake.Session{} }
func (fakeChatManager) GetSessionHistory(string) ([]chatintake.Message, error) {
	return []chatintake.Message{}, nil
}
func (fakeChatManager) DeleteSession(string) error         { return nil }
func (fakeChatManager) ListAgents() []chatintake.AgentInfo { return []chatintake.AgentInfo{} }

func drainJobQueue(server *Server) {
	for {
		select {
		case job := <-server.jobQueue:
			job()
		default:
			return
		}
	}
}

func TestGatewayAdditionalCoverage_JobQueueAndAudit(t *testing.T) {
	server := newSplitAPITestServer(t)

	runtimeJob := &state.Job{
		ID:     "job-refresh",
		Kind:   "runtimes.refresh.batch",
		Status: "queued",
		Payload: map[string]any{
			"items": []any{map[string]any{"agent": "main", "workspace": ""}},
		},
	}
	server.enqueueJobFromPayload(runtimeJob)
	drainJobQueue(server)
	if runtimeJob.Status == "" {
		t.Fatal("expected runtime refresh job status to be updated")
	}

	moveJob := &state.Job{
		ID:     "job-move",
		Kind:   "sessions.move.batch",
		Status: "queued",
		Payload: map[string]any{
			"session_ids": []any{"s1"},
			"org":         "",
			"project":     "",
			"workspace":   "missing",
		},
	}
	server.enqueueJobFromPayload(moveJob)
	if moveJob.Status != "failed" {
		t.Fatalf("expected session move job to fail, got %q", moveJob.Status)
	}

	retrySource := &state.Job{ID: "retry-source", Kind: "noop", Status: "failed", Summary: "retry me", Retriable: true, Payload: map[string]any{}}
	if err := server.store.AppendJob(retrySource); err != nil {
		t.Fatalf("append retry source: %v", err)
	}

	cancelJob := &state.Job{ID: "cancel-me", Kind: "noop", Status: "queued", Cancellable: true}
	if err := server.store.AppendJob(cancelJob); err != nil {
		t.Fatalf("append cancel job: %v", err)
	}

	rec := httptest.NewRecorder()
	server.handleAudit(rec, newAdminRequest(http.MethodGet, "/audit", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /audit = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleJobs(rec, newAdminRequest(http.MethodGet, "/jobs", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /jobs = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleJobByID(rec, newAdminRequest(http.MethodGet, "/jobs/retry-source", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /jobs/id = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleCancelJob(rec, newAdminRequest(http.MethodPost, "/jobs/cancel", `{"job_id":"cancel-me"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /jobs/cancel = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	server.handleRetryJob(rec, newAdminRequest(http.MethodPost, "/jobs/retry", `{"job_id":"retry-source"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /jobs/retry = %d body=%s", rec.Code, rec.Body.String())
	}
	drainJobQueue(server)
}

func TestGatewayAdditionalCoverage_CronAndSkills(t *testing.T) {
	server := newSplitAPITestServer(t)
	cronInitOnce = sync.Once{}
	cronScheduler = nil
	server.initCronScheduler()
	defer func() {
		if cronScheduler != nil {
			cronScheduler.Stop()
		}
		cronInitOnce = sync.Once{}
		cronScheduler = nil
	}()

	rec := httptest.NewRecorder()
	server.handleCronList(rec, newAdminRequest(http.MethodGet, "/cron", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleCronList(rec, newAdminRequest(http.MethodPost, "/cron", `{"name":"hourly","schedule":"@hourly","command":"echo hi"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /cron = %d body=%s", rec.Code, rec.Body.String())
	}

	var created map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created cron task: %v", err)
	}
	taskID := created["id"]

	rec = httptest.NewRecorder()
	server.handleCronByID(rec, newAdminRequest(http.MethodGet, "/cron/"+taskID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/id = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleCronByID(rec, newAdminRequest(http.MethodPut, "/cron/"+taskID, `{"name":"hourly-updated","schedule":"@hourly","command":"echo ok"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /cron/id = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	server.handleCronStats(rec, newAdminRequest(http.MethodGet, "/cron/stats", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/stats = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleCronByID(rec, newAdminRequest(http.MethodDelete, "/cron/"+taskID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /cron/id = %d body=%s", rec.Code, rec.Body.String())
	}

	server.mainRuntime.Config.Agent.Skills = nil
	if got := server.currentEnabledSkillCount(); got < 0 {
		t.Fatalf("unexpected skill count %d", got)
	}

	server.mainRuntime.Config.Agent.Skills = []config.AgentSkillRef{{Name: "alpha", Enabled: true}, {Name: "alpha", Enabled: true}, {Name: "beta", Enabled: false}}
	if got := server.currentEnabledSkillCount(); got != 1 {
		t.Fatalf("expected deduplicated enabled skill count, got %d", got)
	}
	_ = server.currentConfiguredSkillRefs()
}

func TestGatewayAdditionalCoverage_ChatV2AndHelpers(t *testing.T) {
	server := newSplitAPITestServer(t)

	rec := httptest.NewRecorder()
	server.handleV2Chat(rec, newAdminRequest(http.MethodPost, "/v2/chat", `{}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST /v2/chat without module = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleV2ChatSessions(rec, newAdminRequest(http.MethodGet, "/v2/chat/sessions", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /v2/chat/sessions without module = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleV2ChatSessionByID(rec, newAdminRequest(http.MethodGet, "/v2/chat/sessions/x", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /v2/chat/sessions/id without module = %d", rec.Code)
	}

	server.chatModule = fakeChatManager{}
	rec = httptest.NewRecorder()
	server.handleV2Chat(rec, newAdminRequest(http.MethodPost, "/v2/chat", `{"agent_name":"helper","message":"hello"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v2/chat = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	server.handleV2ChatSessions(rec, newAdminRequest(http.MethodGet, "/v2/chat/sessions", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v2/chat/sessions = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleV2ChatSessionByID(rec, newAdminRequest(http.MethodDelete, "/v2/chat/sessions/demo", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /v2/chat/sessions/id = %d body=%s", rec.Code, rec.Body.String())
	}

	server.store.AppendTask(&state.Task{ID: "task-1", Workspace: "default", Status: "queued", Assistant: "helper"})
	rec = httptest.NewRecorder()
	server.handleV2Tasks(rec, newAdminRequest(http.MethodGet, "/v2/tasks?workspace=default&status=queued&assistant=helper", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v2/tasks filtered = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	server.handleV2TaskByID(rec, newAdminRequest(http.MethodGet, "/v2/tasks/", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /v2/tasks/ = %d body=%s", rec.Code, rec.Body.String())
	}

	if _, _, _, err := server.v2TaskHierarchy(newAdminRequest(http.MethodGet, "/v2/tasks", ""), "missing-session"); err == nil {
		t.Fatal("expected missing session hierarchy error")
	}
}
