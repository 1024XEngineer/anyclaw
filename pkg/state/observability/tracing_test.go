package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordingExporter struct {
	spans    []*Span
	shutdown bool
}

func (e *recordingExporter) ExportSpans(ctx context.Context, spans []*Span) error {
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *recordingExporter) Shutdown(ctx context.Context) error {
	e.shutdown = true
	return nil
}

func TestTraceProviderLifecycleAndMiddleware(t *testing.T) {
	exporter := &recordingExporter{}
	provider := NewTraceProvider("svc", exporter)

	root, rootCtx := provider.StartSpan(context.Background(), "root",
		WithKind("server"),
		WithAttributes(map[string]any{"kind": "root"}),
	)
	child, childCtx := provider.StartChildSpan(rootCtx, "child")
	if SpanFromContext(childCtx) != child {
		t.Fatal("expected child span to be attached to context")
	}
	if child.ParentSpanID != root.SpanID || child.TraceID != root.TraceID {
		t.Fatalf("expected child span to inherit trace info, got root=%+v child=%+v", root, child)
	}

	child.SetAttribute("key", "value")
	child.AddEvent("evt", "x", 1, "odd")
	child.SetStatus("ok")
	child.RecordError(errors.New("boom"))
	child.End()
	root.End()

	if err := provider.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(exporter.spans) != 2 {
		t.Fatalf("expected 2 exported spans, got %d", len(exporter.spans))
	}
	if exporter.spans[1].Status != "error" {
		t.Fatalf("expected error status after RecordError, got %+v", exporter.spans[1])
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	TraceMiddleware(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SpanFromContext(r.Context()) == nil {
			t.Fatal("expected request context to carry a span")
		}
		w.WriteHeader(http.StatusInternalServerError)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected middleware response code: %d", rec.Code)
	}

	if err := provider.Flush(context.Background()); err != nil {
		t.Fatalf("Flush after middleware: %v", err)
	}
	found := false
	for _, span := range exporter.spans {
		if span.Name == "/boom" {
			found = true
			if span.Status != "error" {
				t.Fatalf("expected middleware span to be marked error, got %+v", span)
			}
		}
	}
	if !found {
		t.Fatal("expected exported middleware span for /boom")
	}

	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !exporter.shutdown {
		t.Fatal("expected exporter shutdown to be called")
	}

	nilExporterProvider := NewTraceProvider("svc", nil)
	if nilExporterProvider.exporter == nil {
		t.Fatal("expected nil exporter to fall back to NoopExporter")
	}
	if err := (ConsoleExporter{}).ExportSpans(context.Background(), exporter.spans); err != nil {
		t.Fatalf("ConsoleExporter.ExportSpans: %v", err)
	}
	if err := (ConsoleExporter{}).Shutdown(context.Background()); err != nil {
		t.Fatalf("ConsoleExporter.Shutdown: %v", err)
	}
}
