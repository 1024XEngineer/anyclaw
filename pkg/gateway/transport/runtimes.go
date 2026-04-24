package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
	runtimepkg "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type RuntimeAuditRecorder func(ctx context.Context, action string, target string, meta map[string]any)
type RuntimeJobEnqueuer func(job func())
type RuntimeCancelChecker func(jobID string) bool
type RuntimeIDGenerator func(prefix string) string

type RuntimePoolAdmin interface {
	List() []runtimepkg.RuntimeInfo
	Metrics() runtimepkg.RuntimeMetrics
	Refresh(agent string, org string, project string, workspace string)
	Status() runtimepkg.PoolStatus
}

type RuntimeGovernanceStore interface {
	AppendJob(job *state.Job) error
	UpdateJob(job *state.Job) error
	ListEvents(limit int) []*state.Event
	ListToolActivities(limit int, sessionID string) []*state.ToolActivityRecord
	ListJobs(limit int) []*state.Job
}

type ControlPlaneSnapshot struct {
	Status         Status                      `json:"status"`
	Channels       []inputlayer.Status         `json:"channels"`
	Runtimes       []runtimepkg.RuntimeInfo    `json:"runtimes"`
	RuntimeMetrics runtimepkg.RuntimeMetrics   `json:"runtime_metrics"`
	RecentEvents   []*state.Event              `json:"recent_events"`
	RecentTools    []*state.ToolActivityRecord `json:"recent_tools"`
	RecentJobs     []*state.Job                `json:"recent_jobs"`
	UpdatedAt      string                      `json:"updated_at"`
}

type RuntimeGovernanceAPI struct {
	Status         StatusDeps
	RuntimePool    RuntimePoolAdmin
	Store          RuntimeGovernanceStore
	AppendAudit    RuntimeAuditRecorder
	EnqueueJob     RuntimeJobEnqueuer
	ShouldCancel   RuntimeCancelChecker
	NextID         RuntimeIDGenerator
	JobMaxAttempts int
	WriteJSON      SessionJSONWriter
}

func (api RuntimeGovernanceAPI) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.RuntimePool == nil {
		api.writeJSON(w, http.StatusOK, []runtimepkg.RuntimeInfo{})
		return
	}
	api.audit(r.Context(), "runtimes.read", "runtimes", nil)
	api.writeJSON(w, http.StatusOK, api.RuntimePool.List())
}

func (api RuntimeGovernanceAPI) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Agent     string `json:"agent"`
		Org       string `json:"org"`
		Project   string `json:"project"`
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if api.RuntimePool != nil {
		api.RuntimePool.Refresh(req.Agent, req.Org, req.Project, req.Workspace)
	}
	api.audit(r.Context(), "runtimes.refresh", req.Workspace, map[string]any{"agent": req.Agent, "org": req.Org, "project": req.Project})
	api.writeJSON(w, http.StatusOK, map[string]any{"status": "refreshed"})
}

func (api RuntimeGovernanceAPI) HandleRefreshBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Items []struct {
			Agent     string `json:"agent"`
			Org       string `json:"org"`
			Project   string `json:"project"`
			Workspace string `json:"workspace"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if api.Store == nil || api.EnqueueJob == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job queue not available"})
		return
	}
	payload := map[string]any{"items": req.Items}
	job := &state.Job{
		ID:          api.nextID("job"),
		Kind:        "runtimes.refresh.batch",
		Status:      "queued",
		Summary:     fmt.Sprintf("Refreshing %d runtimes", len(req.Items)),
		CreatedAt:   time.Now().UTC(),
		Payload:     payload,
		MaxAttempts: api.JobMaxAttempts,
	}
	job.Cancellable = true
	job.Retriable = true
	_ = api.Store.AppendJob(job)

	api.EnqueueJob(func() {
		if api.shouldCancel(job.ID) {
			return
		}
		job.Attempts++
		job.Status = "running"
		job.StartedAt = time.Now().UTC().Format(time.RFC3339)
		_ = api.Store.UpdateJob(job)

		results := make([]map[string]any, 0, len(req.Items))
		failedCount := 0
		for _, item := range req.Items {
			if api.shouldCancel(job.ID) {
				job.Status = "cancelled"
				job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
				job.Cancellable = false
				job.Retriable = true
				job.Details = map[string]any{"results": results}
				_ = api.Store.UpdateJob(job)
				return
			}
			status := map[string]any{"agent": item.Agent, "org": item.Org, "project": item.Project, "workspace": item.Workspace, "status": "refreshed"}
			if strings.TrimSpace(item.Workspace) == "" {
				status["status"] = "failed"
				status["error"] = "workspace is required"
				failedCount++
			} else if api.RuntimePool != nil {
				api.RuntimePool.Refresh(item.Agent, item.Org, item.Project, item.Workspace)
			}
			results = append(results, status)
		}
		if failedCount == len(req.Items) && len(req.Items) > 0 {
			job.Status = "failed"
			job.Error = "all runtime refresh items failed"
			if job.Attempts < job.MaxAttempts {
				job.Status = "queued"
			}
		} else {
			job.Status = "completed"
		}
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Cancellable = false
		job.Retriable = true
		job.Details = map[string]any{"results": results, "failed_count": failedCount}
		_ = api.Store.UpdateJob(job)
	})

	api.audit(r.Context(), "runtimes.refresh.batch", "runtimes", map[string]any{"count": len(req.Items)})
	api.writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "job_id": job.ID, "count": len(req.Items)})
}

func (api RuntimeGovernanceAPI) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.RuntimePool == nil {
		api.writeJSON(w, http.StatusOK, runtimepkg.RuntimeMetrics{})
		return
	}
	api.writeJSON(w, http.StatusOK, api.RuntimePool.Metrics())
}

func (api RuntimeGovernanceAPI) HandleControlPlane(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api.audit(r.Context(), "control-plane.read", "control-plane", nil)
	api.writeJSON(w, http.StatusOK, api.Snapshot())
}

func (api RuntimeGovernanceAPI) Snapshot() ControlPlaneSnapshot {
	channels := []inputlayer.Status{}
	if api.Status.Channels != nil {
		channels = api.Status.Channels.Statuses()
	}

	runtimes := []runtimepkg.RuntimeInfo{}
	metrics := runtimepkg.RuntimeMetrics{}
	if api.RuntimePool != nil {
		runtimes = api.RuntimePool.List()
		metrics = api.RuntimePool.Metrics()
	}

	var recentEvents []*state.Event
	var recentTools []*state.ToolActivityRecord
	var recentJobs []*state.Job
	if api.Store != nil {
		recentEvents = api.Store.ListEvents(24)
		recentTools = api.Store.ListToolActivities(24, "")
		recentJobs = api.Store.ListJobs(12)
	}

	return ControlPlaneSnapshot{
		Status:         StatusSnapshot(api.Status),
		Channels:       channels,
		Runtimes:       runtimes,
		RuntimeMetrics: metrics,
		RecentEvents:   recentEvents,
		RecentTools:    recentTools,
		RecentJobs:     recentJobs,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
}

func (api RuntimeGovernanceAPI) audit(ctx context.Context, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(ctx, action, target, meta)
}

func (api RuntimeGovernanceAPI) shouldCancel(jobID string) bool {
	if api.ShouldCancel == nil {
		return false
	}
	return api.ShouldCancel(jobID)
}

func (api RuntimeGovernanceAPI) nextID(prefix string) string {
	if api.NextID == nil {
		return state.UniqueID(prefix)
	}
	return api.NextID(prefix)
}

func (api RuntimeGovernanceAPI) writeJSON(w http.ResponseWriter, statusCode int, value any) {
	if api.WriteJSON != nil {
		api.WriteJSON(w, statusCode, value)
		return
	}
	writeJSON(w, statusCode, value)
}
