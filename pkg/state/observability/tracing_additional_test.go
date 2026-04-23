package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTraceMiddlewareCapturesServerErrors(t *testing.T) {
	exporter := &recordingExporter{}
	provider := NewTraceProvider("svc", exporter)

	handler := TraceMiddleware(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := exporter.exportedCount(); got != 1 {
		t.Fatalf("expected 1 exported span, got %d", got)
	}
	span := exporter.exportedSpan(0)
	if got := span.Status; got != "error" {
		t.Fatalf("expected error span status, got %q", got)
	}
	if got := span.Attributes["http.status_code"]; got != http.StatusServiceUnavailable {
		t.Fatalf("expected status code attribute 503, got %#v", got)
	}
}

func TestTraceProviderChildSpansAndHelpers(t *testing.T) {
	exporter := &recordingExporter{}
	provider := NewTraceProvider("svc", exporter)

	parent, ctx := provider.StartSpan(context.Background(), "parent")
	child, _ := provider.StartChildSpan(ctx, "child")
	if child.TraceID != parent.TraceID {
		t.Fatalf("expected child trace id %q, got %q", parent.TraceID, child.TraceID)
	}
	if child.ParentSpanID != parent.SpanID {
		t.Fatalf("expected child parent span id %q, got %q", parent.SpanID, child.ParentSpanID)
	}

	if err := provider.Flush(context.Background()); err != nil {
		t.Fatalf("expected empty flush to succeed, got %v", err)
	}

	if got := cloneAnyMap(nil); got != nil {
		t.Fatalf("expected nil any map clone, got %#v", got)
	}
	if got := cloneSpanEvents(nil); got != nil {
		t.Fatalf("expected nil span event clone, got %#v", got)
	}

	eventClone := cloneSpanEvents([]SpanEvent{{
		Name:       "event",
		Attributes: map[string]any{"key": "value"},
	}})
	eventClone[0].Attributes["key"] = "mutated"
	if got := cloneSpanEvents([]SpanEvent{{Name: "event", Attributes: map[string]any{"key": "value"}}})[0].Attributes["key"]; got != "value" {
		t.Fatalf("expected cloned span events to be isolated, got %#v", got)
	}
}

func TestSpanEndIsIdempotent(t *testing.T) {
	exporter := &recordingExporter{}
	provider := NewTraceProvider("svc", exporter)

	span, _ := provider.StartSpan(context.Background(), "request")
	span.End()
	span.End()

	if got := exporter.exportedCount(); got != 1 {
		t.Fatalf("expected span to be exported once, got %d", got)
	}
}

func TestTracingHelpersAndExporters(t *testing.T) {
	provider := NewTraceProvider("svc", nil)
	if _, ok := provider.exporter.(NoopExporter); !ok {
		t.Fatalf("expected nil exporter to fall back to NoopExporter, got %T", provider.exporter)
	}

	span, ctx := provider.StartSpan(context.Background(), "request")
	span.AddEvent("started", "step", 1)
	span.RecordError(errors.New("boom"))
	if got := span.Status; got != "error" {
		t.Fatalf("expected recorded error to mark span as error, got %q", got)
	}
	if len(span.Events) < 2 {
		t.Fatalf("expected events to be recorded, got %d", len(span.Events))
	}

	if got := SpanFromContext(ctx); got != span {
		t.Fatalf("expected span in context, got %p want %p", got, span)
	}
	if got := SpanFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil span from empty context, got %p", got)
	}

	if err := provider.Flush(context.Background()); err != nil {
		t.Fatalf("expected noop flush to succeed, got %v", err)
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected noop shutdown to succeed, got %v", err)
	}

	console := ConsoleExporter{}
	if err := console.ExportSpans(context.Background(), []*Span{{TraceID: "t1", SpanID: "s1", Name: "request"}}); err != nil {
		t.Fatalf("expected console exporter to succeed, got %v", err)
	}
	if err := console.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected console exporter shutdown to succeed, got %v", err)
	}

	var noop NoopExporter
	if err := noop.ExportSpans(context.Background(), nil); err != nil {
		t.Fatalf("expected noop exporter export to succeed, got %v", err)
	}
	if err := noop.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected noop exporter shutdown to succeed, got %v", err)
	}
}
