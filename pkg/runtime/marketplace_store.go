package runtime

import "strings"

func marketplaceStoreRoot(workDir, workingDir string) string {
	if root := strings.TrimSpace(workDir); root != "" {
		return root
	}
	if root := strings.TrimSpace(workingDir); root != "" {
		return root
	}
	return "."
}
