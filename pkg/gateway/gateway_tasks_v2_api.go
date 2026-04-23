package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) handleV2Agents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, s.v2VisibleAgents())
}

func (s *Server) handleV2Tasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks := s.v2ListTasks(r)
		writeJSON(w, http.StatusOK, tasks)
	case http.MethodPost:
		s.handleV2TaskCreate(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleV2TaskCreate(w http.ResponseWriter, r *http.Request) {
	req, commandReq, err := s.surfaceService().DecodeHTTPV2TaskCreate(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	mode, err := gatewaycommands.ValidateV2TaskCreate(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	dispatch, err := s.commandIntakeService().Dispatch(commandReq)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if dispatch.Kind != "mutate" || dispatch.Target != "tasks" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unexpected command dispatch"})
		return
	}

	selectedAgents, err := s.mainEntryPolicy().NormalizeSelectionList(append([]string{req.SelectedAgent}, req.SelectedAgents...)...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	assistantName, err := s.mainEntryPolicy().NormalizeRequestedAgent(req.SelectedAgent, req.Assistant)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	orgID, projectID, workspaceID, err := s.v2TaskHierarchy(r, req.SessionID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errSessionNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	task, err := s.tasks.Create(taskrunner.CreateOptions{
		Title:     req.Title,
		Input:     req.Input,
		Assistant: assistantName,
		Org:       orgID,
		Project:   projectID,
		Workspace: workspaceID,
		SessionID: req.SessionID,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if req.Sync {
		result, err := s.tasks.Execute(r.Context(), task.ID)
		if err != nil {
			if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
				response := s.taskResponse(result.Task, result.Session)
				response["status"] = "waiting_approval"
				response["requested_mode"] = mode
				response["routing_mode"] = "main_agent"
				response["selected_agents"] = selectedAgents
				writeJSON(w, http.StatusAccepted, response)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"task":  task,
				"error": err.Error(),
			})
			return
		}
		s.recordTaskCompletion(result, "task_api_v2")
		response := s.taskResponse(result.Task, result.Session)
		response["requested_mode"] = mode
		response["routing_mode"] = "main_agent"
		response["selected_agents"] = selectedAgents
		writeJSON(w, http.StatusOK, response)
		return
	}

	taskID := task.ID
	go s.executeTaskAsync(taskID, "task_api_v2_async")

	response := s.taskResponse(task, nil)
	response["status"] = "queued"
	response["requested_mode"] = mode
	response["routing_mode"] = "main_agent"
	response["selected_agents"] = selectedAgents
	writeJSON(w, http.StatusAccepted, response)
}

func (s *Server) handleV2TaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	taskID := strings.TrimPrefix(r.URL.Path, "/v2/tasks/")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id required"})
		return
	}

	task, ok := s.tasks.Get(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	writeJSON(w, http.StatusOK, s.taskResponse(task, nil))
}

var errSessionNotFound = errors.New("session not found")

func (s *Server) v2ListTasks(r *http.Request) []*state.Task {
	items := s.store.ListTasks()
	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	assistant := strings.TrimSpace(r.URL.Query().Get("assistant"))
	if workspace == "" && status == "" && assistant == "" {
		return items
	}
	filtered := make([]*state.Task, 0, len(items))
	for _, task := range items {
		if workspace != "" && !strings.EqualFold(strings.TrimSpace(task.Workspace), workspace) {
			continue
		}
		if status != "" && !strings.EqualFold(strings.TrimSpace(task.Status), status) {
			continue
		}
		if assistant != "" && !strings.EqualFold(strings.TrimSpace(task.Assistant), assistant) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered
}

func (s *Server) v2TaskHierarchy(r *http.Request, sessionID string) (string, string, string, error) {
	if sessionID != "" {
		session, ok := s.sessions.Get(sessionID)
		if !ok {
			return "", "", "", errSessionNotFound
		}
		orgID, projectID, workspaceID := state.SessionExecutionHierarchy(session)
		return orgID, projectID, workspaceID, nil
	}
	queryOrg, queryProject, queryWorkspace := s.resolveHierarchyFromQuery(r)
	org, project, workspace, err := s.validateResourceSelection(queryOrg, queryProject, queryWorkspace)
	if err != nil {
		return "", "", "", err
	}
	return org.ID, project.ID, workspace.ID, nil
}

func (s *Server) executeTaskAsync(taskID string, source string) {
	if s == nil || s.tasks == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	result, err := s.tasks.Execute(context.Background(), taskID)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			return
		}
		return
	}
	s.recordTaskCompletion(result, source)
}

func (s *Server) v2VisibleAgents() []map[string]any {
	items := make([]map[string]any, 0, len(s.mainRuntime.Config.Agent.Profiles)+1)
	seen := make(map[string]struct{}, len(s.mainRuntime.Config.Agent.Profiles)+1)
	appendProfile := func(name string, description string, role string, persona string, domain string, expertise []string, entry string, publicEntry bool, routingDecision string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		record := map[string]any{
			"name":             name,
			"description":      strings.TrimSpace(description),
			"role":             strings.TrimSpace(role),
			"persona":          strings.TrimSpace(persona),
			"domain":           strings.TrimSpace(domain),
			"expertise":        expertise,
			"entry":            entry,
			"public_entry":     publicEntry,
			"routing_decision": routingDecision,
		}
		items = append(items, record)
	}

	if profile, ok := s.mainRuntime.Config.ResolveMainAgentProfile(); ok {
		appendProfile(profile.Name, profile.Description, profile.Role, profile.Persona, profile.Domain, append([]string(nil), profile.Expertise...), "main", true, "main_agent")
	} else if mainName := strings.TrimSpace(s.mainRuntime.Config.ResolveMainAgentName()); mainName != "" {
		appendProfile(mainName, s.mainRuntime.Config.Agent.Description, "", "", "", nil, "main", true, "main_agent")
	}

	for _, profile := range s.mainRuntime.Config.Agent.Profiles {
		if !profile.IsEnabled() {
			continue
		}
		appendProfile(profile.Name, profile.Description, profile.Role, profile.Persona, profile.Domain, append([]string(nil), profile.Expertise...), "specialist", false, "main_agent_handoff")
	}
	return items
}
