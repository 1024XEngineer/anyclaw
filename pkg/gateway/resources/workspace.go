package resources

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func normalizeWorkspacePath(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if os.PathSeparator == '\\' {
		return strings.ToLower(clean)
	}
	return clean
}

func EnsureDefaultWorkspace(store *state.Store, workingDir string) error {
	if store == nil {
		return nil
	}
	orgID, projectID, workspaceID := DefaultIDs(workingDir)
	if err := store.UpsertOrg(&state.Org{ID: orgID, Name: "Local Org"}); err != nil {
		return err
	}
	if err := store.UpsertProject(&state.Project{ID: projectID, OrgID: orgID, Name: "Local Project"}); err != nil {
		return err
	}
	desired := &state.Workspace{
		ID:        workspaceID,
		ProjectID: projectID,
		Name:      filepath.Base(workingDir),
		Path:      workingDir,
	}
	if existing, ok := store.GetWorkspace(workspaceID); ok {
		if existing.ProjectID == desired.ProjectID &&
			existing.Name == desired.Name &&
			normalizeWorkspacePath(existing.Path) == normalizeWorkspacePath(desired.Path) {
			return nil
		}
		existing.ProjectID = desired.ProjectID
		existing.Name = desired.Name
		existing.Path = desired.Path
		return store.UpsertWorkspace(existing)
	}
	for _, existing := range store.ListWorkspaces() {
		if existing.ProjectID != projectID {
			continue
		}
		samePath := normalizeWorkspacePath(existing.Path) == normalizeWorkspacePath(desired.Path)
		sameName := existing.Name == desired.Name
		if !samePath && !sameName {
			continue
		}
		if existing.ID != desired.ID {
			if err := store.RebindWorkspaceID(existing.ID, desired.ID); err != nil {
				return err
			}
		}
		existing.ID = desired.ID
		existing.ProjectID = desired.ProjectID
		existing.Name = desired.Name
		existing.Path = desired.Path
		return store.UpsertWorkspace(existing)
	}
	return store.UpsertWorkspace(desired)
}
