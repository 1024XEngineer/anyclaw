package plugin

import (
	"fmt"
	"strings"
	"sync"
)

type CapabilityIndex struct {
	mu sync.RWMutex

	byPlugin   map[string]*PluginCapabilities
	byTag      map[string][]string
	byPlatform map[string][]string
	byKind     map[string][]string
	byAction   map[string][]string

	workflows map[string]WorkflowCapability
	tools     map[string]ToolCapability
	nodes     map[string]NodeCapability
	surfaces  map[string]SurfaceCapability
	apps      map[string]AppCapability
}

type PluginCapabilities struct {
	PluginID     string
	Name         string
	Description  string
	Kinds        []string
	Tags         []string
	Platforms    []string
	Capabilities []string

	Tools     []ToolCapability
	Workflows []WorkflowCapability
	Nodes     []NodeCapability
	Surfaces  []SurfaceCapability
	Apps      []AppCapability
}

type WorkflowCapability struct {
	PluginID    string
	Name        string
	Action      string
	Tags        []string
	Description string
	InputSchema map[string]any
}

type ToolCapability struct {
	PluginID    string
	Name        string
	Category    string
	Description string
	InputSchema map[string]any
}

type NodeCapability struct {
	PluginID     string
	Name         string
	Platforms    []string
	Capabilities []string
	Actions      []NodeActionSpecV2
}

type SurfaceCapability struct {
	PluginID     string
	Name         string
	Path         string
	Capabilities []string
}

type AppCapability struct {
	PluginID  string
	Name      string
	Platforms []string
	Actions   []AppActionSpecV2
	Workflows []AppWorkflowSpecV2
}

func NewCapabilityIndex() *CapabilityIndex {
	return &CapabilityIndex{
		byPlugin:   make(map[string]*PluginCapabilities),
		byTag:      make(map[string][]string),
		byPlatform: make(map[string][]string),
		byKind:     make(map[string][]string),
		byAction:   make(map[string][]string),

		workflows: make(map[string]WorkflowCapability),
		tools:     make(map[string]ToolCapability),
		nodes:     make(map[string]NodeCapability),
		surfaces:  make(map[string]SurfaceCapability),
		apps:      make(map[string]AppCapability),
	}
}

func (ci *CapabilityIndex) Index(manifest *ManifestV2) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	capabilities := &PluginCapabilities{
		PluginID:     manifest.PluginID,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Kinds:        manifest.Kinds,
		Tags:         manifest.CapabilityTags,
		Platforms:    manifest.Platforms,
		Capabilities: manifest.CapabilityTags,
	}

	ci.byPlugin[manifest.PluginID] = capabilities

	for _, kind := range manifest.Kinds {
		ci.byKind[kind] = append(ci.byKind[kind], manifest.PluginID)
	}

	for _, platform := range manifest.Platforms {
		ci.byPlatform[platform] = append(ci.byPlatform[platform], manifest.PluginID)
	}

	for _, tag := range manifest.CapabilityTags {
		ci.byTag[tag] = append(ci.byTag[tag], manifest.PluginID)
	}

	if manifest.Tool != nil {
		toolCap := ToolCapability{
			PluginID:    manifest.PluginID,
			Name:        manifest.Tool.Name,
			Category:    manifest.Tool.Category,
			Description: manifest.Tool.Description,
			InputSchema: manifest.Tool.InputSchema,
		}
		capabilities.Tools = append(capabilities.Tools, toolCap)
		ci.tools[manifest.Tool.Name] = toolCap
		ci.byAction[manifest.Tool.Name] = append(ci.byAction[manifest.Tool.Name], manifest.PluginID)
	}

	if manifest.App != nil {
		appCap := AppCapability{
			PluginID:  manifest.PluginID,
			Name:      manifest.App.Name,
			Platforms: manifest.App.Platforms,
			Actions:   manifest.App.Actions,
			Workflows: manifest.App.Workflows,
		}
		capabilities.Apps = append(capabilities.Apps, appCap)
		ci.apps[manifest.App.Name] = appCap

		for _, action := range manifest.App.Actions {
			actionKey := fmt.Sprintf("%s:%s", manifest.App.Name, action.Name)
			ci.byAction[actionKey] = append(ci.byAction[actionKey], manifest.PluginID)
		}

		for _, workflow := range manifest.App.Workflows {
			workflowCap := WorkflowCapability{
				PluginID:    manifest.PluginID,
				Name:        workflow.Name,
				Action:      workflow.Action,
				Tags:        workflow.Tags,
				Description: workflow.Description,
				InputSchema: workflow.InputSchema,
			}
			capabilities.Workflows = append(capabilities.Workflows, workflowCap)
			workflowKey := fmt.Sprintf("%s:%s", manifest.App.Name, workflow.Name)
			ci.workflows[workflowKey] = workflowCap

			for _, tag := range workflow.Tags {
				ci.byTag[tag] = append(ci.byTag[tag], manifest.PluginID)
			}
		}
	}

	if manifest.Node != nil {
		nodeCap := NodeCapability{
			PluginID:     manifest.PluginID,
			Name:         manifest.Node.Name,
			Platforms:    manifest.Node.Platforms,
			Capabilities: manifest.Node.Capabilities,
			Actions:      manifest.Node.Actions,
		}
		capabilities.Nodes = append(capabilities.Nodes, nodeCap)
		ci.nodes[manifest.Node.Name] = nodeCap
	}

	if manifest.Surface != nil {
		surfaceCap := SurfaceCapability{
			PluginID:     manifest.PluginID,
			Name:         manifest.Surface.Name,
			Path:         manifest.Surface.Path,
			Capabilities: manifest.Surface.Capabilities,
		}
		capabilities.Surfaces = append(capabilities.Surfaces, surfaceCap)
		ci.surfaces[manifest.Surface.Name] = surfaceCap
	}

	return nil
}

func (ci *CapabilityIndex) Remove(pluginID string) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	capabilities, ok := ci.byPlugin[pluginID]
	if !ok {
		return
	}

	for _, kind := range capabilities.Kinds {
		ci.removeFromMap(ci.byKind[kind], pluginID)
	}
	for _, platform := range capabilities.Platforms {
		ci.removeFromMap(ci.byPlatform[platform], pluginID)
	}
	for _, tag := range capabilities.Tags {
		ci.removeFromMap(ci.byTag[tag], pluginID)
	}

	for _, tool := range capabilities.Tools {
		delete(ci.tools, tool.Name)
		ci.removeFromMap(ci.byAction[tool.Name], pluginID)
	}

	for _, app := range capabilities.Apps {
		delete(ci.apps, app.Name)
		for _, action := range app.Actions {
			actionKey := fmt.Sprintf("%s:%s", app.Name, action.Name)
			ci.removeFromMap(ci.byAction[actionKey], pluginID)
		}
		for _, workflow := range app.Workflows {
			workflowKey := fmt.Sprintf("%s:%s", app.Name, workflow.Name)
			delete(ci.workflows, workflowKey)
		}
	}

	for _, node := range capabilities.Nodes {
		delete(ci.nodes, node.Name)
	}

	for _, surface := range capabilities.Surfaces {
		delete(ci.surfaces, surface.Name)
	}

	delete(ci.byPlugin, pluginID)
}

func (ci *CapabilityIndex) removeFromMap(slice []string, item string) {
	for i, v := range slice {
		if v == item {
			_ = append(slice[:i], slice[i+1:]...)
			break
		}
	}
}

func (ci *CapabilityIndex) GetByPlugin(pluginID string) (*PluginCapabilities, bool) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	cap, ok := ci.byPlugin[pluginID]
	return cap, ok
}

func (ci *CapabilityIndex) GetByTag(tag string) []string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	result := make([]string, len(ci.byTag[tag]))
	copy(result, ci.byTag[tag])
	return result
}

func (ci *CapabilityIndex) GetByPlatform(platform string) []string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	result := make([]string, len(ci.byPlatform[platform]))
	copy(result, ci.byPlatform[platform])
	return result
}

func (ci *CapabilityIndex) GetByKind(kind string) []string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	result := make([]string, len(ci.byKind[kind]))
	copy(result, ci.byKind[kind])
	return result
}

func (ci *CapabilityIndex) SearchWorkflows(query string, limit int) []WorkflowCapability {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	query = strings.ToLower(query)
	var results []WorkflowCapability

	for _, wf := range ci.workflows {
		if strings.Contains(strings.ToLower(wf.Name), query) ||
			strings.Contains(strings.ToLower(wf.Description), query) {
			results = append(results, wf)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results
}

func (ci *CapabilityIndex) GetAll() map[string]*PluginCapabilities {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	result := make(map[string]*PluginCapabilities)
	for k, v := range ci.byPlugin {
		result[k] = v
	}
	return result
}
