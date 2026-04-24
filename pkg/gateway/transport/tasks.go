package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	gatewayintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type TaskPermissionChecker func(ctx context.Context, permission string) bool
type TaskAuditRecorder func(ctx context.Context, action string, target string, meta map[string]any)
type TaskAgentResolver func(name string) (string, error)
type TaskSelectionResolver func(r *http.Request) (*state.Org, *state.Project, *state.Workspace, error)
type TaskResponseBuilder func(task *state.Task, session *state.Session) map[string]any
type TaskCompletionRecorder func(result *taskrunner.ExecutionResult, source string)
type TaskJSONWriter func(w http.ResponseWriter, statusCode int, value any)

type TaskLister interface {
	List() []*state.Task
	Get(id string) (*state.Task, bool)
	Steps(taskID string) []*state.TaskStep
	Create(opts taskrunner.CreateOptions) (*state.Task, error)
	Execute(ctx context.Context, taskID string) (*taskrunner.ExecutionResult, error)
}

type TaskAPI struct {
	Tasks               TaskLister
	Sessions            *state.SessionManager
	CheckPermission     TaskPermissionChecker
	AppendAudit         TaskAuditRecorder
	NormalizeEntryAgent TaskAgentResolver
	ResolveSelection    TaskSelectionResolver
	BuildResponse       TaskResponseBuilder
	RecordCompletion    TaskCompletionRecorder
	WriteJSON           TaskJSONWriter
}

func (api TaskAPI) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleList(w, r)
	case http.MethodPost:
		api.handleCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api TaskAPI) HandleByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")
	path = strings.TrimSpace(path)
	if path == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}
	if api.Tasks == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task manager not available"})
		return
	}

	parts := strings.Split(path, "/")
	taskID := strings.TrimSpace(parts[0])
	task, ok := api.Tasks.Get(taskID)
	if !ok {
		api.writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if len(parts) > 1 && parts[1] == "steps" {
		api.handleSteps(w, r, taskID)
		return
	}
	if len(parts) > 1 && parts[1] == "execute" {
		api.handleExecute(w, r, taskID, task)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !api.allowed(r.Context(), "tasks.read") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.read"})
		return
	}

	api.audit(r.Context(), "tasks.read", taskID, nil)
	api.writeJSON(w, http.StatusOK, api.response(task, nil))
}

func (api TaskAPI) handleList(w http.ResponseWriter, r *http.Request) {
	if api.Tasks == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task manager not available"})
		return
	}
	if !api.allowed(r.Context(), "tasks.read") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.read"})
		return
	}

	items := api.Tasks.List()
	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	filtered := make([]*state.Task, 0, len(items))
	for _, task := range items {
		if workspace != "" && task.Workspace != workspace {
			continue
		}
		if status != "" && !strings.EqualFold(task.Status, status) {
			continue
		}
		filtered = append(filtered, task)
	}

	api.audit(r.Context(), "tasks.read", "tasks", map[string]any{"count": len(filtered)})
	api.writeJSON(w, http.StatusOK, filtered)
}

func (api TaskAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	if api.Tasks == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task manager not available"})
		return
	}
	if !api.allowed(r.Context(), "tasks.write") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.write"})
		return
	}

	var req struct {
		Title     string `json:"title"`
		Input     string `json:"input"`
		Agent     string `json:"agent"`
		Assistant string `json:"assistant"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}

	assistantName, err := api.normalizeEntryAgent(gatewayintake.RequestedAgentName(req.Agent, req.Assistant))
	if err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var orgID, projectID, workspaceID string
	if strings.TrimSpace(req.SessionID) != "" {
		if api.Sessions == nil {
			api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session manager not available"})
			return
		}
		session, ok := api.Sessions.Get(strings.TrimSpace(req.SessionID))
		if !ok {
			api.writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		orgID, projectID, workspaceID = state.SessionExecutionHierarchy(session)
	} else {
		org, project, workspace, err := api.resolveSelection(r)
		if err != nil {
			api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		orgID, projectID, workspaceID = org.ID, project.ID, workspace.ID
	}

	task, err := api.Tasks.Create(taskrunner.CreateOptions{
		Title:     req.Title,
		Input:     req.Input,
		Assistant: assistantName,
		Org:       orgID,
		Project:   projectID,
		Workspace: workspaceID,
		SessionID: req.SessionID,
	})
	if err != nil {
		api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	result, err := api.Tasks.Execute(r.Context(), task.ID)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			api.audit(r.Context(), "tasks.write", task.ID, map[string]any{"status": "waiting_approval"})
			response := api.response(taskResultTask(result, task), taskResultSession(result))
			response["status"] = "waiting_approval"
			api.writeJSON(w, http.StatusAccepted, response)
			return
		}
		api.audit(r.Context(), "tasks.write", task.ID, map[string]any{"status": "failed"})
		api.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "task": task})
		return
	}

	api.recordCompletion(result, "task_api")
	api.audit(r.Context(), "tasks.write", task.ID, map[string]any{"status": taskResultTask(result, task).Status})
	api.writeJSON(w, http.StatusCreated, api.response(taskResultTask(result, task), taskResultSession(result)))
}

func (api TaskAPI) handleSteps(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !api.allowed(r.Context(), "tasks.read") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.read"})
		return
	}
	api.writeJSON(w, http.StatusOK, api.Tasks.Steps(taskID))
}

func (api TaskAPI) handleExecute(w http.ResponseWriter, r *http.Request, taskID string, task *state.Task) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !api.allowed(r.Context(), "tasks.write") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.write"})
		return
	}

	result, err := api.Tasks.Execute(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			api.audit(r.Context(), "tasks.write", taskID, map[string]any{"status": "waiting_approval", "resume": true})
			response := api.response(taskResultTask(result, task), taskResultSession(result))
			response["status"] = "waiting_approval"
			api.writeJSON(w, http.StatusAccepted, response)
			return
		}
		api.audit(r.Context(), "tasks.write", taskID, map[string]any{"status": "failed", "resume": true})
		api.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "task": task})
		return
	}

	api.recordCompletion(result, "task_resume")
	api.audit(r.Context(), "tasks.write", taskID, map[string]any{"status": taskResultTask(result, task).Status, "resume": true})
	api.writeJSON(w, http.StatusOK, api.response(taskResultTask(result, task), taskResultSession(result)))
}

func taskResultTask(result *taskrunner.ExecutionResult, fallback *state.Task) *state.Task {
	if result != nil && result.Task != nil {
		return result.Task
	}
	return fallback
}

func taskResultSession(result *taskrunner.ExecutionResult) *state.Session {
	if result == nil {
		return nil
	}
	return result.Session
}

func (api TaskAPI) normalizeEntryAgent(name string) (string, error) {
	if api.NormalizeEntryAgent == nil {
		return strings.TrimSpace(name), nil
	}
	return api.NormalizeEntryAgent(name)
}

func (api TaskAPI) resolveSelection(r *http.Request) (*state.Org, *state.Project, *state.Workspace, error) {
	if api.ResolveSelection == nil {
		return &state.Org{}, &state.Project{}, &state.Workspace{}, nil
	}
	return api.ResolveSelection(r)
}

func (api TaskAPI) allowed(ctx context.Context, permission string) bool {
	if api.CheckPermission == nil {
		return true
	}
	return api.CheckPermission(ctx, permission)
}

func (api TaskAPI) audit(ctx context.Context, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(ctx, action, target, meta)
}

func (api TaskAPI) response(task *state.Task, session *state.Session) map[string]any {
	if api.BuildResponse != nil {
		return api.BuildResponse(task, session)
	}
	response := map[string]any{
		"task": task,
	}
	if task != nil && api.Tasks != nil {
		response["steps"] = api.Tasks.Steps(task.ID)
	}
	if session != nil {
		response["session"] = session
	}
	return response
}

func (api TaskAPI) recordCompletion(result *taskrunner.ExecutionResult, source string) {
	if api.RecordCompletion == nil {
		return
	}
	api.RecordCompletion(result, source)
}

func (api TaskAPI) writeJSON(w http.ResponseWriter, statusCode int, value any) {
	if api.WriteJSON != nil {
		api.WriteJSON(w, statusCode, value)
		return
	}
	writeJSON(w, statusCode, value)
}
