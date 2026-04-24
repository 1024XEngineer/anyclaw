package resources

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func ValidateSelection(store *state.Store, orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error) {
	var org *state.Org
	var project *state.Project
	var workspace *state.Workspace
	var ok bool
	if workspaceID == "" {
		return nil, nil, nil, fmt.Errorf("workspace is required")
	}
	if store == nil {
		return nil, nil, nil, fmt.Errorf("store is not initialized")
	}
	workspace, ok = store.GetWorkspace(workspaceID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	project, ok = store.GetProject(workspace.ProjectID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("project not found: %s", workspace.ProjectID)
	}
	org, ok = store.GetOrg(project.OrgID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("org not found: %s", project.OrgID)
	}
	if projectID != "" && project.ID != projectID {
		return nil, nil, nil, fmt.Errorf("workspace %s does not belong to project %s", workspaceID, projectID)
	}
	if orgID != "" && org.ID != orgID {
		return nil, nil, nil, fmt.Errorf("workspace %s does not belong to org %s", workspaceID, orgID)
	}
	return org, project, workspace, nil
}

func DefaultIDs(workingDir string) (string, string, string) {
	workspaceID := "workspace-default"
	clean := strings.TrimSpace(strings.ToLower(workingDir))
	if clean != "" {
		replacer := strings.NewReplacer(":", "-", "\\", "-", "/", "-", " ", "-")
		clean = replacer.Replace(clean)
		clean = strings.Trim(clean, "-.")
		if clean != "" {
			workspaceID = "ws-" + clean
		}
	}
	return "org-local", "project-local", workspaceID
}
