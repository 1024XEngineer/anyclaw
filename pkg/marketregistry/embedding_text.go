package marketregistry

import (
	"fmt"
	"strings"
)

func buildArtifactEmbeddingText(artifact Artifact) string {
	lines := []string{
		fmt.Sprintf("kind: %s", strings.TrimSpace(string(artifact.Kind))),
		fmt.Sprintf("name: %s", strings.TrimSpace(artifact.Name)),
		fmt.Sprintf("summary: %s", strings.TrimSpace(artifact.Summary)),
		fmt.Sprintf("description: %s", strings.TrimSpace(artifact.DescriptionMD)),
		fmt.Sprintf("publisher: %s", strings.TrimSpace(artifact.Publisher)),
		fmt.Sprintf("tags: %s", strings.Join(trimmedStrings(artifact.Tags), ", ")),
		fmt.Sprintf("hit_signals: %s", strings.Join(trimmedStrings(artifact.HitSignals), ", ")),
		fmt.Sprintf("use_case: %s", strings.TrimSpace(artifact.ManifestSummary["use_case"])),
		fmt.Sprintf("permissions: %s", strings.Join(trimmedStrings(artifact.Permissions), ", ")),
	}
	return strings.Join(compactStrings(lines...), "\n")
}
