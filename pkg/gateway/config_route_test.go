package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigRouteAllowsReadPermission(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/config", server.wrap("/config", server.handleConfigAPI))

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "reader",
		Permissions: []string{"config.read"},
	}))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
