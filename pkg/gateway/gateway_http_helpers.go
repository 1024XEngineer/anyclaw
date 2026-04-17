package gateway

import (
	"context"
	"fmt"
	"net/http"

	gatewayevents "github.com/anyclaw/anyclaw/pkg/gateway/events"
	"github.com/anyclaw/anyclaw/pkg/state"
)

func (s *Server) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if s == nil {
		return
	}
	gatewayevents.AppendEvent(s.store, s.bus, eventType, sessionID, payload)
}

func (s *Server) appendAudit(user *AuthUser, action string, target string, meta map[string]any) {
	if s == nil {
		return
	}
	gatewayevents.AppendAudit(s.store, user, action, target, meta)
}

func sessionCreatedEventPayload(session *state.Session) map[string]any {
	return gatewayevents.SessionCreatedEventPayload(session)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	gatewayevents.HandleList(w, r, s.store)
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	gatewayevents.HandleStream(w, r, s.store, s.bus)
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
	if s.rateLimit != nil {
		next = s.rateLimit.Wrap(next)
	}
	if s.auth != nil {
		return s.auth.Wrap(path, next)
	}
	return next
}

func requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if permission == "" {
			next(w, r)
			return
		}
		user := UserFromContext(r.Context())
		if !HasPermission(user, permission) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": permission})
			return
		}
		next(w, r)
	}
}

func requirePermissionByMethod(methodPermissions map[string]string, defaultPermission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		permission := defaultPermission
		if methodPermissions != nil {
			if mapped, ok := methodPermissions[r.Method]; ok {
				permission = mapped
			}
		}
		if permission == "" {
			next(w, r)
			return
		}
		user := UserFromContext(r.Context())
		if !HasPermission(user, permission) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": permission})
			return
		}
		next(w, r)
	}
}

func requireHierarchyAccess(resolve func(*http.Request) (string, string, string), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, project, workspace := "", "", ""
		if resolve != nil {
			org, project, workspace = resolve(r)
		}
		if org == "" && project == "" && workspace == "" {
			next(w, r)
			return
		}
		if !HasHierarchyAccess(UserFromContext(r.Context()), org, project, workspace) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_org": org, "required_project": project, "required_workspace": workspace})
			return
		}
		next(w, r)
	}
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
