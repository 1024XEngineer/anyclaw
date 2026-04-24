package events

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestHandleList(t *testing.T) {
	store := newTestStore(t)
	AppendEvent(store, nil, "old", "sess-1", nil)
	AppendEvent(store, nil, "new", "sess-2", map[string]any{"x": "y"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events?limit=1", nil)
	HandleList(rec, req, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var events []*state.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(events) != 1 || events[0].Type != "new" {
		t.Fatalf("expected limited latest event, got %#v", events)
	}

	rec = httptest.NewRecorder()
	HandleList(rec, httptest.NewRequest(http.MethodPost, "/events", nil), store)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	HandleList(rec, httptest.NewRequest(http.MethodGet, "/events", nil), nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected empty list for nil store, got code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestWriteSSEEvent(t *testing.T) {
	rec := httptest.NewRecorder()
	event := &state.Event{
		ID:        "evt-1",
		Type:      "gateway.start",
		SessionID: "sess-1",
		Payload:   map[string]any{"ready": true},
	}

	if err := WriteSSEEvent(rec, event); err != nil {
		t.Fatalf("WriteSSEEvent: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id: evt-1\n") || !strings.Contains(body, "event: gateway.start\n") || !strings.Contains(body, `"ready":true`) {
		t.Fatalf("unexpected SSE body: %q", body)
	}

	writer := noFlushWriter{header: http.Header{}}
	if err := WriteSSEEvent(&writer, &state.Event{ID: "fail", Type: "gateway.error"}); err == nil {
		t.Fatal("expected write error")
	}
}

func TestHandleStreamReplaysFilteredEventsAndExitsOnContextDone(t *testing.T) {
	store := newTestStore(t)
	bus := state.NewEventBus()
	AppendEvent(store, nil, "session.message", "sess-1", map[string]any{"body": "hello"})
	AppendEvent(store, nil, "session.message", "sess-2", map[string]any{"body": "skip"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/events/stream?replay=10&session_id=sess-1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	HandleStream(rec, req, store, bus)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "session.message") || !strings.Contains(body, "sess-1") {
		t.Fatalf("expected replayed session event, got %q", body)
	}
	if strings.Contains(body, "sess-2") {
		t.Fatalf("expected session filter to exclude sess-2, got %q", body)
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestHandleStreamRejectsUnavailableInputs(t *testing.T) {
	rec := httptest.NewRecorder()
	HandleStream(rec, httptest.NewRequest(http.MethodPost, "/events/stream", nil), nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	HandleStream(rec, httptest.NewRequest(http.MethodGet, "/events/stream", nil), nil, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	writer := noFlushWriter{header: http.Header{}}
	HandleStream(&writer, httptest.NewRequest(http.MethodGet, "/events/stream", nil), newTestStore(t), state.NewEventBus())
	if writer.status != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher, got %d", writer.status)
	}
}

func TestServiceHTTPFacade(t *testing.T) {
	store := newTestStore(t)
	bus := state.NewEventBus()
	service := NewService(store, bus)
	service.AppendEvent("gateway.start", "", nil)

	rec := httptest.NewRecorder()
	service.HandleList(rec, httptest.NewRequest(http.MethodGet, "/events", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "gateway.start") {
		t.Fatalf("expected event list through facade, code=%d body=%q", rec.Code, rec.Body.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec = httptest.NewRecorder()
	service.HandleStream(rec, httptest.NewRequest(http.MethodGet, "/events/stream?replay=1", nil).WithContext(ctx))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "gateway.start") {
		t.Fatalf("expected stream replay through facade, code=%d body=%q", rec.Code, rec.Body.String())
	}
}

type noFlushWriter struct {
	header http.Header
	status int
	body   strings.Builder
}

func (w *noFlushWriter) Header() http.Header {
	return w.header
}

func (w *noFlushWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *noFlushWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if strings.Contains(string(data), "fail") {
		return 0, errors.New("write failed")
	}
	return w.body.Write(data)
}
