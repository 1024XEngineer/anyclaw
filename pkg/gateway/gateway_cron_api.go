package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	runtimeschedule "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/schedule"
)

var cronScheduler *runtimeschedule.Scheduler
var cronInitOnce sync.Once

func (s *Server) initCronScheduler() {
	cronInitOnce.Do(func() {
		executor := s.mainRuntime.NewCronExecutor()
		cronScheduler = runtimeschedule.NewScheduler(executor)

		persister, err := runtimeschedule.NewFilePersister("")
		if err == nil {
			cronScheduler.SetPersister(persister)
			_ = cronScheduler.LoadPersisted()
		}

		_ = cronScheduler.Start()
	})
}

func (s *Server) handleCronList(w http.ResponseWriter, r *http.Request) {
	s.initCronScheduler()

	switch r.Method {
	case http.MethodGet:
		tasks := cronScheduler.ListTasks()
		s.appendAudit(UserFromContext(r.Context()), "cron.read", "cron", map[string]any{"count": len(tasks)})
		writeJSON(w, http.StatusOK, tasks)
	case http.MethodPost:
		var task runtimeschedule.Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if err := task.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		taskID, err := cronScheduler.AddTask(&task)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		s.appendAudit(UserFromContext(r.Context()), "cron.create", taskID, map[string]any{"name": task.Name, "schedule": task.Schedule})
		writeJSON(w, http.StatusCreated, map[string]string{"id": taskID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCronByID(w http.ResponseWriter, r *http.Request) {
	s.initCronScheduler()

	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/cron/"))
	if path == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	taskID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodGet:
		task, ok := cronScheduler.GetTask(taskID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "cron.read", taskID, nil)
		writeJSON(w, http.StatusOK, task)
	case http.MethodPut:
		var task runtimeschedule.Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		task.ID = taskID
		if err := cronScheduler.UpdateTask(&task); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "cron.update", taskID, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if err := cronScheduler.DeleteTask(taskID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "cron.delete", taskID, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCronStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.initCronScheduler()
	writeJSON(w, http.StatusOK, cronScheduler.Stats())
}
