package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Logger wraps slog.Logger with convenience methods and global access.
type Logger struct {
	*slog.Logger
}

var (
	globalLogger   *Logger
	globalLoggerMu sync.Mutex
)

// Config holds logger configuration.
type Config struct {
	Level  string `json:"level"`
	Format string `json:"format"` // "json" or "text"
	Output string `json:"output"` // "stdout", "stderr", or file path
}

// DefaultConfig returns the default logger config.
func DefaultConfig() Config {
	return Config{
		Level:  "info",
		Format: "json",
		Output: "stderr",
	}
}

// NewLogger creates a new structured logger from config.
func NewLogger(cfg Config) (*Logger, error) {
	level := parseLevel(cfg.Level)

	var w io.Writer
	switch strings.ToLower(cfg.Output) {
	case "stdout":
		w = os.Stdout
	case "stderr":
		w = os.Stderr
	default:
		if cfg.Output != "" {
			f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return nil, err
			}
			w = f
		} else {
			w = os.Stderr
		}
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	switch strings.ToLower(cfg.Format) {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return &Logger{slog.New(handler)}, nil
}

// SetGlobal sets the global logger.
func SetGlobal(l *Logger) {
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()
	globalLogger = l
	slog.SetDefault(l.Logger)
}

// Global returns the global logger.
func Global() *Logger {
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()
	if globalLogger == nil {
		cfg := DefaultConfig()
		l, _ := NewLogger(cfg)
		globalLogger = l
	}
	return globalLogger
}

// With returns a new logger with the given attributes.
func (l *Logger) With(attrs ...any) *Logger {
	return &Logger{l.Logger.With(slogAttrs(attrs)...)}
}

// Trace logs at DEBUG level with "trace" key.
func (l *Logger) Trace(msg string, attrs ...any) {
	l.Debug(msg, append([]any{"event", "trace"}, slogAttrs(attrs)...)...)
}

// Span logs a span start/end.
func (l *Logger) Span(name string, attrs ...any) {
	l.Debug("span", append([]any{"span", name}, slogAttrs(attrs)...)...)
}

// Event logs a structured event.
func (l *Logger) Event(name string, attrs ...any) {
	l.Info("event", append([]any{"event", name}, slogAttrs(attrs)...)...)
}

// Request logs an HTTP request.
func (l *Logger) Request(method, path string, attrs ...any) {
	l.Info("http_request", append([]any{"method", method, "path", path}, slogAttrs(attrs)...)...)
}

// Response logs an HTTP response.
func (l *Logger) Response(method, path string, status int, durationMs int64, attrs ...any) {
	l.Info("http_response", append([]any{
		"method", method,
		"path", path,
		"status", status,
		"duration_ms", durationMs,
	}, slogAttrs(attrs)...)...)
}

// Error logs an error with context.
func (l *Logger) Error(msg string, attrs ...any) {
	l.Logger.Error(msg, slogAttrs(attrs)...)
}

// Warn logs a warning.
func (l *Logger) Warn(msg string, attrs ...any) {
	l.Logger.Warn(msg, slogAttrs(attrs)...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, attrs ...any) {
	l.Logger.Info(msg, slogAttrs(attrs)...)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, attrs ...any) {
	l.Logger.Debug(msg, slogAttrs(attrs)...)
}

func slogAttrs(pairs []any) []any {
	if len(pairs)%2 != 0 {
		pairs = append(pairs, nil)
	}
	return pairs
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug", "trace":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
