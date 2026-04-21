package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func HandleList(w http.ResponseWriter, r *http.Request, store *state.Store) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if store == nil {
		writeJSON(w, http.StatusOK, []*state.Event{})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, store.ListEvents(limit))
}

func HandleStream(w http.ResponseWriter, r *http.Request, store *state.Store, bus *state.EventBus) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if store == nil || bus == nil {
		http.Error(w, "event stream unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("replay")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			limit = parsed
		}
	}
	filterSessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))

	for _, event := range store.ListEvents(limit) {
		if filterSessionID != "" && event.SessionID != filterSessionID {
			continue
		}
		if err := WriteSSEEvent(w, event); err != nil {
			return
		}
	}
	flusher.Flush()

	ch := bus.Subscribe(32)
	defer bus.Unsubscribe(ch)

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case event := <-ch:
			if event == nil {
				continue
			}
			if filterSessionID != "" && event.SessionID != filterSessionID {
				continue
			}
			if err := WriteSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func WriteSSEEvent(w http.ResponseWriter, event *state.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
