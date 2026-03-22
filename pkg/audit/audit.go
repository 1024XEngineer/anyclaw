package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time      string         `json:"time"`
	AgentName string         `json:"agent_name,omitempty"`
	Action    string         `json:"action"`
	Input     map[string]any `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type Logger struct {
	path      string
	agentName string
	mu        sync.Mutex
}

func New(path string, agentName string) *Logger {
	return &Logger{path: path, agentName: agentName}
}

func (l *Logger) LogTool(toolName string, input map[string]any, output string, err error) {
	event := Event{
		Time:      time.Now().Format(time.RFC3339),
		AgentName: l.agentName,
		Action:    toolName,
		Input:     cloneMap(input),
		Output:    output,
	}
	if err != nil {
		event.Error = err.Error()
	}
	_ = l.Append(event)
}

func (l *Logger) Append(event Event) error {
	if l == nil || l.path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, string(data)); err != nil {
		return err
	}
	return nil
}

func (l *Logger) Tail(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 20
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := splitLines(string(data))
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	result := make([]Event, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err == nil {
			result = append(result, event)
		}
	}
	return result, nil
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func splitLines(input string) []string {
	current := ""
	lines := []string{}
	for _, r := range input {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
			continue
		}
		if r != '\r' {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
