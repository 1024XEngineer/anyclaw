package pluginruntime

// Manifest describes a plugin's identity and declared capabilities.
type Manifest struct {
	ID           string
	Version      string
	Capabilities []string
}
