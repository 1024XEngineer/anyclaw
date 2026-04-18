package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "audit.read", "audit", nil)
	writeJSON(w, http.StatusOK, s.store.ListAudit(100))
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "jobs.read", "jobs", nil)
	writeJSON(w, http.StatusOK, s.store.ListJobs(100))
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/jobs/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	job, ok := s.store.GetJob(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "jobs.detail.read", id, nil)
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	job, ok := s.store.GetJob(req.JobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job is not cancellable"})
		return
	}
	s.jobCancel[job.ID] = true
	job.Status = "cancelled"
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	job.Cancellable = false
	job.Retriable = true
	_ = s.store.UpdateJob(job)
	s.appendAudit(UserFromContext(r.Context()), "jobs.cancel", job.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	job, ok := s.store.GetJob(req.JobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if !job.Retriable {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job is not retriable"})
		return
	}
	clone := &state.Job{ID: uniqueID("job"), Kind: job.Kind, Status: "queued", Summary: job.Summary + " (retry)", CreatedAt: time.Now().UTC(), RetryOf: job.ID, Cancellable: true, Retriable: true, Payload: job.Payload}
	_ = s.store.AppendJob(clone)
	s.enqueueJobFromPayload(clone)
	s.appendAudit(UserFromContext(r.Context()), "jobs.retry", job.ID, map[string]any{"new_job": clone.ID})
	writeJSON(w, http.StatusOK, map[string]any{"status": "queued", "job_id": clone.ID})
}
