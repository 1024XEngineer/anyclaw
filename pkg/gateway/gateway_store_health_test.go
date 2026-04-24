package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	agentstore "github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type fakeStoreManager struct {
	packages     map[string]agentstore.AgentPackage
	installErr   error
	uninstallErr error
}

func (f *fakeStoreManager) List(filter agentstore.StoreFilter) []agentstore.AgentPackage {
	items := make([]agentstore.AgentPackage, 0, len(f.packages))
	for _, pkg := range f.packages {
		items = append(items, pkg)
	}
	return items
}

func (f *fakeStoreManager) Get(id string) (*agentstore.AgentPackage, error) {
	pkg, ok := f.packages[id]
	if !ok {
		return nil, fmt.Errorf("package not found: %s", id)
	}
	return &pkg, nil
}

func (f *fakeStoreManager) Search(string) []agentstore.AgentPackage {
	return f.List(agentstore.StoreFilter{})
}
func (f *fakeStoreManager) Install(id string) error {
	if f.installErr != nil {
		return f.installErr
	}
	if _, ok := f.packages[id]; !ok {
		return fmt.Errorf("package not found: %s", id)
	}
	return nil
}
func (f *fakeStoreManager) Uninstall(id string) error {
	if f.uninstallErr != nil {
		return f.uninstallErr
	}
	if _, ok := f.packages[id]; !ok {
		return fmt.Errorf("package not found: %s", id)
	}
	return nil
}
func (f *fakeStoreManager) Installed() []agentstore.AgentPackage {
	return f.List(agentstore.StoreFilter{})
}
func (f *fakeStoreManager) IsInstalled(string) bool { return true }
func (f *fakeStoreManager) GetCategories() []string { return []string{"tool", "assistant"} }
func (f *fakeStoreManager) GetTags() []string       { return []string{"featured", "fast"} }

func TestGatewayStoreAndProviderHealth(t *testing.T) {
	server := newSplitAPITestServer(t)
	server.storeModule = &fakeStoreManager{packages: map[string]agentstore.AgentPackage{
		"pkg-1": {ID: "pkg-1", Name: "One"},
	}}

	t.Run("store list and details", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleV2Store(rec, newAdminRequest(http.MethodGet, "/v2/store?installed=true&q=test", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v2/store = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleV2StoreByID(rec, newAdminRequest(http.MethodGet, "/v2/store/", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v2/store/ = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleV2StoreByID(rec, newAdminRequest(http.MethodGet, "/v2/store/pkg-1", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v2/store/pkg-1 = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleV2StoreByID(rec, newAdminRequest(http.MethodPost, "/v2/store/pkg-1/install", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /v2/store/pkg-1/install = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleV2StoreByID(rec, newAdminRequest(http.MethodPost, "/v2/store/pkg-1/uninstall", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /v2/store/pkg-1/uninstall = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleV2StoreByID(rec, newAdminRequest(http.MethodGet, "/v2/store/missing", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /v2/store/missing = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("provider health", func(t *testing.T) {
		if got := quickProviderHealth(config.ProviderProfile{Enabled: config.BoolPtr(false)}); got.Status != "disabled" {
			t.Fatalf("disabled provider status = %q", got.Status)
		}
		if got := quickProviderHealth(config.ProviderProfile{Enabled: config.BoolPtr(true)}); got.Status != "invalid" {
			t.Fatalf("missing provider status = %q", got.Status)
		}
		if got := quickProviderHealth(config.ProviderProfile{Enabled: config.BoolPtr(true), Provider: "openai"}); got.Status != "missing_key" {
			t.Fatalf("missing key status = %q", got.Status)
		}
		if got := quickProviderHealth(config.ProviderProfile{Enabled: config.BoolPtr(true), Provider: "ollama", BaseURL: "://bad"}); got.Status != "invalid_base_url" {
			t.Fatalf("invalid url status = %q", got.Status)
		}

		successSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer successSrv.Close()
		if got := activeProviderTest(context.Background(), config.ProviderProfile{Enabled: config.BoolPtr(true), Provider: "compatible", APIKey: "key", BaseURL: successSrv.URL}); got.Status != "reachable" {
			t.Fatalf("reachable provider status = %q", got.Status)
		}

		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer authSrv.Close()
		if got := activeProviderTest(context.Background(), config.ProviderProfile{Enabled: config.BoolPtr(true), Provider: "compatible", APIKey: "key", BaseURL: authSrv.URL}); got.Status != "auth_error" {
			t.Fatalf("auth provider status = %q", got.Status)
		}

		notFoundSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer notFoundSrv.Close()
		if got := activeProviderTest(context.Background(), config.ProviderProfile{Enabled: config.BoolPtr(true), Provider: "compatible", APIKey: "key", BaseURL: notFoundSrv.URL}); got.Status != "endpoint_not_found" {
			t.Fatalf("404 provider status = %q", got.Status)
		}

		if got := activeProviderTest(context.Background(), config.ProviderProfile{Enabled: config.BoolPtr(true), Provider: "compatible", APIKey: "key"}); got.Status != "ready" {
			t.Fatalf("default endpoint status = %q", got.Status)
		}
	})
}
