package gateway

import (
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/plugin"
)

type appActionView struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	ToolName    string         `json:"tool_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type appWorkflowView struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Action      string         `json:"action"`
	ToolName    string         `json:"tool_name"`
	Tags        []string       `json:"tags,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`
}

type desktopAppView struct {
	LaunchCommand        string   `json:"launch_command,omitempty"`
	WindowTitle          string   `json:"window_title,omitempty"`
	WindowClass          string   `json:"window_class,omitempty"`
	FocusStrategy        string   `json:"focus_strategy,omitempty"`
	DetectionHints       []string `json:"detection_hints,omitempty"`
	RequiresHostReviewed bool     `json:"requires_host_reviewed,omitempty"`
}

type appView struct {
	Plugin       string            `json:"plugin"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Enabled      bool              `json:"enabled"`
	Builtin      bool              `json:"builtin"`
	Transport    string            `json:"transport,omitempty"`
	Platforms    []string          `json:"platforms,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Desktop      *desktopAppView   `json:"desktop,omitempty"`
	Actions      []appActionView   `json:"actions,omitempty"`
	Workflows    []appWorkflowView `json:"workflows,omitempty"`
}

func appManifestToView(manifest plugin.Manifest) appView {
	view := appView{
		Plugin:  manifest.Name,
		Name:    manifest.Name,
		Enabled: manifest.Enabled,
		Builtin: manifest.Builtin,
	}
	if manifest.App != nil {
		if strings.TrimSpace(manifest.App.Name) != "" {
			view.Name = strings.TrimSpace(manifest.App.Name)
		}
		view.Description = strings.TrimSpace(firstNonEmpty(manifest.App.Description, manifest.Description))
		view.Transport = strings.TrimSpace(manifest.App.Transport)
		view.Platforms = append([]string{}, manifest.App.Platforms...)
		view.Capabilities = append([]string{}, manifest.App.Capabilities...)
		if manifest.App.Desktop != nil {
			view.Desktop = &desktopAppView{
				LaunchCommand:        manifest.App.Desktop.LaunchCommand,
				WindowTitle:          manifest.App.Desktop.WindowTitle,
				WindowClass:          manifest.App.Desktop.WindowClass,
				FocusStrategy:        manifest.App.Desktop.FocusStrategy,
				DetectionHints:       append([]string{}, manifest.App.Desktop.DetectionHints...),
				RequiresHostReviewed: manifest.App.Desktop.RequiresHostReviewed,
			}
			if view.Transport == "" {
				view.Transport = "desktop"
			}
		}
		actions := make([]appActionView, 0, len(manifest.App.Actions))
		for _, action := range manifest.App.Actions {
			if strings.TrimSpace(action.Name) == "" {
				continue
			}
			actions = append(actions, appActionView{
				Name:        strings.TrimSpace(action.Name),
				Description: strings.TrimSpace(action.Description),
				Kind:        strings.TrimSpace(action.Kind),
				ToolName:    plugin.AppActionToolName(manifest.Name, action.Name),
				InputSchema: action.InputSchema,
			})
		}
		view.Actions = actions
		workflows := make([]appWorkflowView, 0, len(manifest.App.Workflows))
		for _, workflow := range manifest.App.Workflows {
			if strings.TrimSpace(workflow.Name) == "" || strings.TrimSpace(workflow.Action) == "" {
				continue
			}
			workflows = append(workflows, appWorkflowView{
				Name:        strings.TrimSpace(workflow.Name),
				Description: strings.TrimSpace(workflow.Description),
				Action:      strings.TrimSpace(workflow.Action),
				ToolName:    plugin.AppWorkflowToolName(manifest.Name, workflow.Name),
				Tags:        append([]string{}, workflow.Tags...),
				InputSchema: workflow.InputSchema,
				Defaults:    workflow.Defaults,
			})
		}
		view.Workflows = workflows
	} else {
		view.Description = strings.TrimSpace(manifest.Description)
	}
	return view
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apps := s.listAppViews()
	s.appendAudit(UserFromContext(r.Context()), "apps.read", "apps", nil)
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) listAppViews() []appView {
	if s == nil || s.plugins == nil {
		return []appView{}
	}
	apps := make([]appView, 0)
	for _, manifest := range s.plugins.List() {
		if manifest.App == nil {
			continue
		}
		apps = append(apps, appManifestToView(manifest))
	}
	return apps
}
