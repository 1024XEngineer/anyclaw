package observability

import (
	"log/slog"
	"testing"
)

func TestLoggerCreationGlobalAndHelpers(t *testing.T) {
	globalLoggerMu.Lock()
	previous := globalLogger
	globalLoggerMu.Unlock()
	t.Cleanup(func() {
		globalLoggerMu.Lock()
		globalLogger = previous
		globalLoggerMu.Unlock()
		if previous != nil {
			slog.SetDefault(previous.Logger)
		}
	})

	logger, err := NewLogger(Config{
		Level:  "debug",
		Format: "text",
		Output: "stderr",
	})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Trace("trace-msg", "key", "value")
	logger.Span("span-name", "key", "value")
	logger.Event("evt", "key", "value")
	logger.Request(httpMethodGet, "/demo", "request_id", "1")
	logger.Response(httpMethodGet, "/demo", 201, 12, "request_id", "1")
	logger.Error("error-msg", "code", 500)
	logger.Warn("warn-msg", "only_key")
	logger.Info("info-msg", "key", "value")
	logger.Debug("debug-msg", "key", "value")
	logger.With("scope", "test").Info("child-msg")

	SetGlobal(logger)
	if Global() != logger {
		t.Fatal("expected global logger to return the logger we set")
	}

	if parseLevel("debug") != slog.LevelDebug || parseLevel("warning") != slog.LevelWarn || parseLevel("error") != slog.LevelError || parseLevel("unknown") != slog.LevelInfo {
		t.Fatal("unexpected parsed levels")
	}
	attrs := slogAttrs([]any{"odd"})
	if len(attrs) != 2 || attrs[1] != nil {
		t.Fatalf("expected odd attrs to be padded, got %+v", attrs)
	}
}

const httpMethodGet = "GET"
