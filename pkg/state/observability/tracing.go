package observability

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// TraceProvider is the entry point for tracing.
type TraceProvider struct {
	serviceName string
	exporter    SpanExporter
	mu          sync.RWMutex
	spans       []*Span
}

// SpanExporter exports completed spans.
type SpanExporter interface {
	ExportSpans(ctx context.Context, spans []*Span) error
	Shutdown(ctx context.Context) error
}

// NoopExporter is an exporter that discards all spans.
type NoopExporter struct{}

func (NoopExporter) ExportSpans(ctx context.Context, spans []*Span) error { return nil }
func (NoopExporter) Shutdown(ctx context.Context) error                   { return nil }

// NewTraceProvider creates a new trace provider.
func NewTraceProvider(serviceName string, exporter SpanExporter) *TraceProvider {
	if exporter == nil {
		exporter = NoopExporter{}
	}
	return &TraceProvider{
		serviceName: serviceName,
		exporter:    exporter,
	}
}

// StartSpan starts a new root span.
func (tp *TraceProvider) StartSpan(ctx context.Context, name string, opts ...SpanOption) (*Span, context.Context) {
	span := newSpan(tp, name, opts...)
	ctx = ContextWithSpan(ctx, span)

	tp.mu.Lock()
	tp.spans = append(tp.spans, span)
	tp.mu.Unlock()

	return span, ctx
}

// StartChildSpan starts a span as a child of the span in the context.
func (tp *TraceProvider) StartChildSpan(ctx context.Context, name string, opts ...SpanOption) (*Span, context.Context) {
	parent := SpanFromContext(ctx)
	if parent != nil {
		opts = append(opts, WithParent(parent))
	}
	return tp.StartSpan(ctx, name, opts...)
}

// Flush exports all completed spans.
func (tp *TraceProvider) Flush(ctx context.Context) error {
	tp.mu.Lock()
	spans := make([]*Span, len(tp.spans))
	copy(spans, tp.spans)
	tp.spans = tp.spans[:0]
	tp.mu.Unlock()

	if len(spans) == 0 {
		return nil
	}
	return tp.exporter.ExportSpans(ctx, spans)
}

// Shutdown shuts down the trace provider.
func (tp *TraceProvider) Shutdown(ctx context.Context) error {
	if err := tp.Flush(ctx); err != nil {
		return err
	}
	return tp.exporter.Shutdown(ctx)
}

// Span represents a single operation in a trace.
type Span struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      *time.Time     `json:"end_time,omitempty"`
	DurationMs   int64          `json:"duration_ms,omitempty"`
	Status       string         `json:"status"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Events       []SpanEvent    `json:"events,omitempty"`
	ServiceName  string         `json:"service_name"`
	provider     *TraceProvider
	mu           sync.Mutex
}

// SpanEvent is an event within a span.
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  time.Time      `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanOption configures a span.
type SpanOption func(*Span)

// WithParent sets the parent span.
func WithParent(parent *Span) SpanOption {
	return func(s *Span) {
		s.TraceID = parent.TraceID
		s.ParentSpanID = parent.SpanID
	}
}

// WithKind sets the span kind.
func WithKind(kind string) SpanOption {
	return func(s *Span) {
		s.Kind = kind
	}
}

// WithAttributes sets initial attributes.
func WithAttributes(attrs map[string]any) SpanOption {
	return func(s *Span) {
		for k, v := range attrs {
			s.Attributes[k] = v
		}
	}
}

var spanCounter atomic.Uint64

func newSpan(tp *TraceProvider, name string, opts ...SpanOption) *Span {
	counter := spanCounter.Add(1)
	now := time.Now().UTC()
	span := &Span{
		TraceID:     fmt.Sprintf("%016x", counter),
		SpanID:      fmt.Sprintf("%016x%04x", counter, time.Now().UnixNano()%0xFFFF),
		Name:        name,
		Kind:        "internal",
		StartTime:   now,
		Status:      "unset",
		Attributes:  make(map[string]any),
		Events:      make([]SpanEvent, 0),
		ServiceName: tp.serviceName,
		provider:    tp,
	}

	for _, opt := range opts {
		opt(span)
	}

	return span
}

// SetAttribute sets an attribute on the span.
func (s *Span) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Attributes[key] = value
}

// AddEvent adds an event to the span.
func (s *Span) AddEvent(name string, attrs ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := SpanEvent{
		Name:      name,
		Timestamp: time.Now().UTC(),
	}
	if len(attrs) > 0 {
		event.Attributes = make(map[string]any)
		for i := 0; i < len(attrs)-1; i += 2 {
			if key, ok := attrs[i].(string); ok {
				event.Attributes[key] = attrs[i+1]
			}
		}
	}
	s.Events = append(s.Events, event)
}

// SetStatus sets the span status.
func (s *Span) SetStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}

// End marks the span as completed.
func (s *Span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.EndTime = &now
	s.DurationMs = now.Sub(s.StartTime).Milliseconds()
}

// RecordError records an error on the span.
func (s *Span) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "error"
	s.Attributes["error.message"] = err.Error()
	s.Events = append(s.Events, SpanEvent{
		Name:      "exception",
		Timestamp: time.Now().UTC(),
		Attributes: map[string]any{
			"exception.message": err.Error(),
			"exception.type":    fmt.Sprintf("%T", err),
		},
	})
}

// spanContextKey is the context key for storing spans.
type spanContextKey struct{}

// ContextWithSpan stores a span in the context.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

// SpanFromContext retrieves a span from the context.
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return span
	}
	return nil
}

// TraceMiddleware creates an HTTP middleware that traces requests.
func TraceMiddleware(tp *TraceProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, ctx := tp.StartSpan(r.Context(), r.URL.Path,
				WithKind("server"),
				WithAttributes(map[string]any{
					"http.method":     r.Method,
					"http.url":        r.URL.String(),
					"http.user_agent": r.UserAgent(),
					"http.client_ip":  r.RemoteAddr,
				}),
			)
			defer span.End()

			r = r.WithContext(ctx)

			sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(sw, r)

			span.SetAttribute("http.status_code", sw.statusCode)
			if sw.statusCode >= 500 {
				span.SetStatus("error")
			}
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}

// ConsoleExporter prints spans to stdout as JSON lines.
type ConsoleExporter struct{}

func (ConsoleExporter) ExportSpans(ctx context.Context, spans []*Span) error {
	for _, span := range spans {
		Global().Info("span_exported",
			"trace_id", span.TraceID,
			"span_id", span.SpanID,
			"name", span.Name,
			"duration_ms", span.DurationMs,
			"status", span.Status,
		)
	}
	return nil
}

func (ConsoleExporter) Shutdown(ctx context.Context) error { return nil }
