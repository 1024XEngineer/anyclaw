package session

// Compactor will own transcript compaction and summarization policies.
type Compactor interface {
	Compact(messages []ChatMessage) ([]ChatMessage, error)
}
