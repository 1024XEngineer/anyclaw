package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type SessionMover interface {
	MoveSession(sessionID string, org string, project string, workspace string, agent string) (*state.Session, error)
}

type SessionMoveSelectionResolver func(orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error)
type SessionMoveAuditRecorder func(ctx context.Context, action string, target string, meta map[string]any)
type SessionMoveRuntimePool interface {
	InvalidateByWorkspace(workspaceID string)
}
type SessionMoveJobStore interface {
	AppendJob(job *state.Job) error
	UpdateJob(job *state.Job) error
}
type SessionMoveJobEnqueuer func(job func())
type SessionMoveCancelChecker func(jobID string) bool
type SessionMoveIDGenerator func(prefix string) string

type SessionMoveAPI struct {
	Sessions         SessionMover
	ResolveSelection SessionMoveSelectionResolver
	RuntimePool      SessionMoveRuntimePool
	AppendAudit      SessionMoveAuditRecorder
	Store            SessionMoveJobStore
	EnqueueJob       SessionMoveJobEnqueuer
	ShouldCancelJob  SessionMoveCancelChecker
	NextID           SessionMoveIDGenerator
	JobMaxAttempts   int
	WriteJSON        SessionJSONWriter
}

func (api SessionMoveAPI) HandleSingle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Org       string `json:"org"`
		Project   string `json:"project"`
		Workspace string `json:"workspace"`
		Agent     string `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	org, project, workspace, err := api.resolveSelection(req.Org, req.Project, req.Workspace)
	if err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := api.Sessions.MoveSession(req.SessionID, org.ID, project.ID, workspace.ID, req.Agent)
	if err != nil {
		api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if api.RuntimePool != nil {
		api.RuntimePool.InvalidateByWorkspace(workspace.ID)
	}
	api.audit(r.Context(), "runtimes.invalidate", workspace.ID, map[string]any{"reason": "session move"})
	api.audit(r.Context(), "sessions.move", req.SessionID, map[string]any{"org": org.ID, "project": project.ID, "workspace": workspace.ID, "agent": req.Agent})
	api.writeJSON(w, http.StatusOK, updated)
}

func (api SessionMoveAPI) HandleBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionIDs []string `json:"session_ids"`
		Org        string   `json:"org"`
		Project    string   `json:"project"`
		Workspace  string   `json:"workspace"`
		Agent      string   `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	org, project, workspace, err := api.resolveSelection(req.Org, req.Project, req.Workspace)
	if err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if api.Store == nil || api.EnqueueJob == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job queue not available"})
		return
	}

	payload := map[string]any{"session_ids": req.SessionIDs, "org": org.ID, "project": project.ID, "workspace": workspace.ID, "agent": req.Agent}
	job := &state.Job{
		ID:          api.nextID("job"),
		Kind:        "sessions.move.batch",
		Status:      "queued",
		Summary:     fmt.Sprintf("Moving %d sessions", len(req.SessionIDs)),
		CreatedAt:   time.Now().UTC(),
		Payload:     payload,
		MaxAttempts: api.JobMaxAttempts,
	}
	job.Cancellable = true
	job.Retriable = true
	_ = api.Store.AppendJob(job)

	api.EnqueueJob(func() {
		if api.shouldCancelJob(job.ID) {
			return
		}
		job.Attempts++
		job.Status = "running"
		job.StartedAt = time.Now().UTC().Format(time.RFC3339)
		_ = api.Store.UpdateJob(job)

		updatedCount := 0
		failedCount := 0
		results := make([]map[string]any, 0, len(req.SessionIDs))
		for _, sessionID := range req.SessionIDs {
			if api.shouldCancelJob(job.ID) {
				job.Status = "cancelled"
				job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
				job.Cancellable = false
				job.Retriable = true
				job.Details = map[string]any{"results": results, "target_workspace": workspace.ID}
				_ = api.Store.UpdateJob(job)
				return
			}
			if _, err := api.Sessions.MoveSession(sessionID, org.ID, project.ID, workspace.ID, req.Agent); err == nil {
				updatedCount++
				results = append(results, map[string]any{"session_id": sessionID, "status": "moved"})
			} else {
				failedCount++
				results = append(results, map[string]any{"session_id": sessionID, "status": "failed", "error": err.Error()})
			}
		}

		if updatedCount > 0 && api.RuntimePool != nil {
			api.RuntimePool.InvalidateByWorkspace(workspace.ID)
		}
		if failedCount == len(req.SessionIDs) && len(req.SessionIDs) > 0 {
			job.Status = "failed"
			job.Error = "all session move items failed"
			if job.Attempts < job.MaxAttempts {
				job.Status = "queued"
			}
		} else {
			job.Status = "completed"
		}
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Cancellable = false
		job.Retriable = true
		job.Details = map[string]any{"results": results, "target_workspace": workspace.ID, "failed_count": failedCount}
		_ = api.Store.UpdateJob(job)
		api.audit(r.Context(), "sessions.move.batch", workspace.ID, map[string]any{"count": updatedCount, "agent": req.Agent})
	})

	api.writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "job_id": job.ID, "count": len(req.SessionIDs)})
}

func (api SessionMoveAPI) resolveSelection(orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error) {
	if api.ResolveSelection == nil {
		return &state.Org{}, &state.Project{}, &state.Workspace{}, nil
	}
	return api.ResolveSelection(orgID, projectID, workspaceID)
}

func (api SessionMoveAPI) audit(ctx context.Context, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(ctx, action, target, meta)
}

func (api SessionMoveAPI) shouldCancelJob(jobID string) bool {
	if api.ShouldCancelJob == nil {
		return false
	}
	return api.ShouldCancelJob(jobID)
}

func (api SessionMoveAPI) nextID(prefix string) string {
	if api.NextID == nil {
		return state.UniqueID(prefix)
	}
	return api.NextID(prefix)
}

func (api SessionMoveAPI) writeJSON(w http.ResponseWriter, statusCode int, value any) {
	if api.WriteJSON != nil {
		api.WriteJSON(w, statusCode, value)
		return
	}
	writeJSON(w, statusCode, value)
}
