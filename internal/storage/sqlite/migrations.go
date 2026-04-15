package sqlite

// Migration represents one schema migration step.
type Migration struct {
	Version int
	Name    string
}
