package scheduleui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimeschedule "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/schedule"
)

func TestRegisterUIHandlerServesCronViews(t *testing.T) {
	scheduler := runtimeschedule.New()
	mux := http.NewServeMux()
	RegisterUIHandler(mux, scheduler, "/cron")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cron/?json=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/?json=1 = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cron/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/stats = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cron/validate?expr=*+*+*+*+*", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/validate = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cron/next?expr=*+*+*+*+*", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/next = %d", rec.Code)
	}

	body := bytes.NewBufferString(`{"name":"sample","schedule":"invalid","command":"echo hi","enabled":true}`)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/cron/", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /cron/ = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cron/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/ = %d", rec.Code)
	}
}
