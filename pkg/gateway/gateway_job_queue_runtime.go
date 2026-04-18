package gateway

import (
	"fmt"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) enqueueJobFromPayload(job *state.Job) {
	if job == nil {
		return
	}
	switch job.Kind {
	case "runtimes.refresh.batch":
		s.enqueueRuntimeRefreshBatch(job)
	case "sessions.move.batch":
		s.enqueueSessionMoveBatch(job)
	}
}

func (s *Server) enqueueRuntimeRefreshBatch(job *state.Job) {
	rawItems, _ := job.Payload["items"].([]any)
	items := make([]struct {
		Agent     string `json:"agent"`
		Org       string `json:"org"`
		Project   string `json:"project"`
		Workspace string `json:"workspace"`
	}, 0, len(rawItems))
	for _, raw := range rawItems {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, struct {
			Agent     string `json:"agent"`
			Org       string `json:"org"`
			Project   string `json:"project"`
			Workspace string `json:"workspace"`
		}{
			Agent: fmt.Sprint(m["Agent"], m["agent"]),
			Org:   fmt.Sprint(m["Org"], m["org"]),
			Project: fmt.Sprint(m["Project"],
				m["project"]),
			Workspace: fmt.Sprint(m["Workspace"], m["workspace"]),
		})
	}
	s.jobQueue <- func() {
		job.Status = "running"
		job.StartedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.store.UpdateJob(job)
		results := make([]map[string]any, 0, len(items))
		failedCount := 0
		for _, item := range items {
			if strings.TrimSpace(item.Workspace) == "" {
				results = append(results, map[string]any{"agent": item.Agent, "org": item.Org, "project": item.Project, "workspace": item.Workspace, "status": "failed", "error": "workspace is required"})
				failedCount++
				continue
			}
			s.runtimePool.Refresh(item.Agent, item.Org, item.Project, item.Workspace)
			results = append(results, map[string]any{"agent": item.Agent, "org": item.Org, "project": item.Project, "workspace": item.Workspace, "status": "refreshed"})
		}
		if failedCount == len(items) && len(items) > 0 {
			job.Status = "failed"
			job.Error = "all runtime refresh items failed"
		} else {
			job.Status = "completed"
		}
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Cancellable = false
		job.Details = map[string]any{"results": results, "failed_count": failedCount}
		_ = s.store.UpdateJob(job)
	}
}

func (s *Server) enqueueSessionMoveBatch(job *state.Job) {
	rawIDs, _ := job.Payload["session_ids"].([]any)
	sessionIDs := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		sessionIDs = append(sessionIDs, fmt.Sprint(raw))
	}
	orgID := fmt.Sprint(job.Payload["org"])
	projectID := fmt.Sprint(job.Payload["project"])
	workspaceID := fmt.Sprint(job.Payload["workspace"])
	agent := fmt.Sprint(job.Payload["agent"])
	org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
	if err != nil {
		job.Status = "failed"
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Error = err.Error()
		_ = s.store.UpdateJob(job)
		return
	}
	s.jobQueue <- func() {
		job.Status = "running"
		job.StartedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.store.UpdateJob(job)
		updatedCount := 0
		failedCount := 0
		results := make([]map[string]any, 0, len(sessionIDs))
		for _, sessionID := range sessionIDs {
			if _, err := s.sessions.MoveSession(sessionID, org.ID, project.ID, workspace.ID, agent); err == nil {
				updatedCount++
				results = append(results, map[string]any{"session_id": sessionID, "status": "moved"})
			} else {
				failedCount++
				results = append(results, map[string]any{"session_id": sessionID, "status": "failed", "error": err.Error()})
			}
		}
		if updatedCount > 0 {
			s.runtimePool.InvalidateByWorkspace(workspace.ID)
		}
		if failedCount == len(sessionIDs) && len(sessionIDs) > 0 {
			job.Status = "failed"
			job.Error = "all session move items failed"
		} else {
			job.Status = "completed"
		}
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Cancellable = false
		job.Details = map[string]any{"results": results, "target_workspace": workspace.ID, "failed_count": failedCount}
		_ = s.store.UpdateJob(job)
	}
}
