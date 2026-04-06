package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/anyclaw/anyclaw/pkg/cron"
)

// Webhook and Node handlers

func (s *Server) handleWebhookIncoming(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		http.NotFound(w, r)
		return
	}

	statusCode, body := s.webhooks.HandleRequest(r.Context(), r, func(ctx context.Context, webhook *Webhook, payload []byte) (string, error) {
		// Process webhook through agent
		agentName := webhook.Agent
		if agentName == "" {
			agentName = s.app.Config.ResolveMainAgentName()
		}

		targetApp, err := s.runtimePool.GetOrCreate(agentName, "", "", "")
		if err != nil {
			return "", err
		}

		// Format message from webhook
		message := fmt.Sprintf("[Webhook: %s] %s", webhook.Name, string(payload))
		if webhook.Template != "" {
			message = fmt.Sprintf("%s\n\nPayload:\n%s", webhook.Template, string(payload))
		}

		targetApp.Agent.SetHistory(nil)
		response, err := targetApp.Agent.Run(ctx, message)
		if err != nil {
			return "", err
		}

		s.appendEvent("webhook.triggered", "", map[string]any{
			"webhook_id": webhook.ID,
			"name":       webhook.Name,
			"response":   response,
		})

		return response, nil
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

func (s *Server) handleNodesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.nodes == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	nodes := s.nodes.List()
	s.appendAudit(UserFromContext(r.Context()), "nodes.read", "nodes", map[string]any{"count": len(nodes)})
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleNodeByID(w http.ResponseWriter, r *http.Request) {
	if s.nodes == nil {
		http.NotFound(w, r)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/nodes/")
	path = strings.TrimSpace(path)
	if path == "" {
		http.Error(w, "node id required", http.StatusBadRequest)
		return
	}

	parts := strings.Split(path, "/")
	nodeID := parts[0]

	switch r.Method {
	case http.MethodGet:
		node, ok := s.nodes.Get(nodeID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "nodes.read", nodeID, nil)
		writeJSON(w, http.StatusOK, node)

	case http.MethodDelete:
		if err := s.nodes.Unregister(nodeID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "nodes.delete", nodeID, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNodeInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.nodes == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no nodes available"})
		return
	}

	var req struct {
		NodeID string         `json:"node_id"`
		Action string         `json:"action"`
		Params map[string]any `json:"params,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.NodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_id is required"})
		return
	}

	if req.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action is required"})
		return
	}

	result, err := s.nodes.Invoke(r.Context(), req.NodeID, req.Action, req.Params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.appendAudit(UserFromContext(r.Context()), "nodes.invoke", req.NodeID, map[string]any{"action": req.Action})
	writeJSON(w, http.StatusOK, result)
}

// Cron handlers

var cronScheduler *cron.Scheduler
var cronInitOnce sync.Once

func (s *Server) initCronScheduler() {
	cronInitOnce.Do(func() {
		executor := cron.NewAgentExecutor(s.app.Agent, s.app.Orchestrator)
		cronScheduler = cron.NewScheduler(executor)

		persister, err := cron.NewFilePersister("")
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
		var task cron.Task
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

	path := strings.TrimPrefix(r.URL.Path, "/cron/")
	path = strings.TrimSpace(path)
	if path == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	parts := strings.Split(path, "/")
	taskID := parts[0]

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
		var task cron.Task
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

// handleNodesHealth returns node manager health
func (s *Server) handleNodesHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.nodes == nil {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "online": 0, "offline": 0})
		return
	}

	writeJSON(w, http.StatusOK, s.nodes.Health())
}

// handleCronStats returns cron scheduler statistics
func (s *Server) handleCronStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.initCronScheduler()

	writeJSON(w, http.StatusOK, cronScheduler.Stats())
}

// Device Pairing handlers

func (s *Server) handleDevicePairing(w http.ResponseWriter, r *http.Request) {
	if s.devicePairing == nil {
		http.Error(w, "device pairing not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		action := r.URL.Query().Get("action")
		if action == "list" || action == "" {
			devices := s.devicePairing.ListPaired()
			writeJSON(w, http.StatusOK, map[string]any{"devices": devices, "status": s.devicePairing.GetStatus()})
			return
		}
		if action == "status" {
			writeJSON(w, http.StatusOK, s.devicePairing.GetStatus())
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid action"})

	case http.MethodPost:
		var req PairingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		resp, err := s.devicePairing.HandleRequest(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !resp.OK {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": resp.Error})
			return
		}
		writeJSON(w, http.StatusOK, resp)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDevicePairingCode(w http.ResponseWriter, r *http.Request) {
	if s.devicePairing == nil {
		http.Error(w, "device pairing not initialized", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DeviceName string `json:"device_name"`
		DeviceType string `json:"device_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	code, err := s.devicePairing.GeneratePairingCode(req.DeviceName, req.DeviceType)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    code.Code,
		"expires": code.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		"device":  code.DeviceName,
		"type":    code.DeviceType,
	})
}
