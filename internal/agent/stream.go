package agent

// EventType identifies one of the major agent runtime stream families.
type EventType string

const (
	EventLifecycle EventType = "lifecycle"
	EventAssistant EventType = "assistant"
	EventTool      EventType = "tool"
)

// Event is the normalized stream item emitted by the agent runtime.
type Event struct {
	RunID string
	Type  EventType
	Name  string
	Data  map[string]any
}
