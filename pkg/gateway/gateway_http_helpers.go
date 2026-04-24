package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	gatewayevents "github.com/1024XEngineer/anyclaw/pkg/gateway/events"
	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", err.Error())
	}
}

func (s *Server) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if s == nil {
		return
	}
	s.eventsService().AppendEvent(eventType, sessionID, payload)
}

func (s *Server) appendAudit(user *AuthUser, action string, target string, meta map[string]any) {
	if s == nil {
		return
	}
	s.eventsService().AppendAudit(user, action, target, meta)
}

func sessionCreatedEventPayload(session *state.Session) map[string]any {
	return gatewayevents.SessionCreatedEventPayload(session)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	s.eventsService().HandleList(w, r)
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	s.eventsService().HandleStream(w, r)
}

func (s *Server) startWorkers(ctx context.Context) {
	workerCount := s.mainRuntime.Config.Gateway.JobWorkerCount
	if workerCount <= 0 {
		workerCount = 1
	}
	for i := 0; i < workerCount; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-s.jobQueue:
					if job != nil {
						job()
					}
				}
			}
		}()
	}
}

func (s *Server) shouldCancelJob(id string) bool {
	return s.jobCancel[id]
}

func (s *Server) wrap(path string, next http.HandlerFunc) http.HandlerFunc {
	return s.governanceService().Wrap(path, next)
}

func requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return gatewayRequirePermission(permission, next)
}

func requirePermissionByMethod(methodPermissions map[string]string, defaultPermission string, next http.HandlerFunc) http.HandlerFunc {
	return gatewayRequirePermissionByMethod(methodPermissions, defaultPermission, next)
}

func requireHierarchyAccess(resolve func(*http.Request) (string, string, string), next http.HandlerFunc) http.HandlerFunc {
	return gatewayRequireHierarchyAccess(resolve, next)
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return defaultVal
	}
	return n
}

func gatewayRequirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	service := gatewaygovernance.Service{CurrentUser: UserFromContext}
	return service.RequirePermission(permission, next)
}

func gatewayRequirePermissionByMethod(methodPermissions map[string]string, defaultPermission string, next http.HandlerFunc) http.HandlerFunc {
	service := gatewaygovernance.Service{CurrentUser: UserFromContext}
	return service.RequirePermissionByMethod(methodPermissions, defaultPermission, next)
}

func gatewayRequireHierarchyAccess(resolve func(*http.Request) (string, string, string), next http.HandlerFunc) http.HandlerFunc {
	service := gatewaygovernance.Service{CurrentUser: UserFromContext}
	return service.RequireHierarchyAccess(resolve, next)
}
