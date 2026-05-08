package marketplace

import (
	"context"
	"errors"
	"sort"
	"strings"

	agentstore "github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/clihub"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

var ErrArtifactNotFound = errors.New("artifact not found")

type LocalCatalogDeps struct {
	Config      *config.Config
	Skills      *skills.SkillsManager
	Plugins     *plugin.Registry
	AgentStore  agentstore.StoreManager
	CLIHub      *clihub.Catalog
	WorkspaceID string
}

type LocalCatalog struct {
	deps LocalCatalogDeps
}

func NewLocalCatalog(deps LocalCatalogDeps) *LocalCatalog {
	return &LocalCatalog{deps: deps}
}

func (c *LocalCatalog) List(ctx context.Context, filter Filter) (ListResult, error) {
	_ = ctx
	items := c.collect(filter)
	items = filterArtifacts(items, filter)
	sortArtifacts(items)

	total := len(items)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	end := offset + limit
	if end > total {
		end = total
	}

	return ListResult{
		Items:  append([]Artifact(nil), items[offset:end]...),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (c *LocalCatalog) Get(ctx context.Context, id string) (*Artifact, error) {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrArtifactNotFound
	}
	for _, item := range c.collect(Filter{}) {
		if strings.EqualFold(item.ID, id) {
			copy := item
			return &copy, nil
		}
	}
	return nil, ErrArtifactNotFound
}

func (c *LocalCatalog) Versions(ctx context.Context, id string) ([]ArtifactVersion, error) {
	artifact, err := c.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(artifact.Version) == "" {
		return nil, nil
	}
	return []ArtifactVersion{{
		Version:       artifact.Version,
		Compatibility: artifact.Compatibility,
	}}, nil
}

func (c *LocalCatalog) collect(filter Filter) []Artifact {
	var items []Artifact
	if filter.Source != "" && filter.Source != SourceLocal {
		return items
	}

	if filter.Kind == "" || filter.Kind == ArtifactKindAgent {
		items = append(items, c.agentProfileArtifacts()...)
		items = append(items, c.agentStoreArtifacts()...)
	}
	if filter.Kind == "" || filter.Kind == ArtifactKindSkill {
		items = append(items, c.skillArtifacts()...)
	}
	if filter.Kind == "" || filter.Kind == ArtifactKindCLI {
		items = append(items, c.cliArtifacts()...)
	}
	if filter.Kind == "" || filter.Kind == ArtifactKindPlugin {
		items = append(items, c.pluginArtifacts()...)
	}
	return dedupeArtifacts(items)
}

func (c *LocalCatalog) agentProfileArtifacts() []Artifact {
	cfg := c.deps.Config
	if cfg == nil {
		return nil
	}

	active := strings.TrimSpace(cfg.ResolveMainAgentName())
	items := make([]Artifact, 0, len(cfg.Agent.Profiles)+1)
	for _, profile := range cfg.Agent.Profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			continue
		}
		enabled := profile.IsEnabled()
		isActive := strings.EqualFold(name, active)
		status := StatusBound
		if isActive {
			status = StatusActive
		} else if !enabled {
			status = StatusDisabled
		}
		items = append(items, Artifact{
			ID:           artifactID(ArtifactKindAgent, name),
			Kind:         ArtifactKindAgent,
			Name:         name,
			DisplayName:  name,
			Description:  firstNonEmpty(profile.Description, profile.Persona, profile.Role),
			Source:       SourceLocal,
			SourceID:     "agent.profiles",
			Status:       status,
			Installed:    true,
			Bound:        true,
			Active:       isActive,
			Enabled:      enabled,
			Category:     firstNonEmpty(profile.Domain, profile.Role, "agent"),
			Tags:         appendUnique(profile.Expertise, profile.Role, profile.Domain),
			Permissions:  agentPermissions(profile),
			TargetHints:  []string{"main_agent", "persistent_subagent", "workspace"},
			Capabilities: agentCapabilities(profile),
			Metadata: map[string]string{
				"provider_ref": profile.ProviderRef,
				"working_dir":  profile.WorkingDir,
			},
		})
	}

	if len(items) == 0 && strings.TrimSpace(cfg.Agent.Name) != "" {
		name := strings.TrimSpace(cfg.Agent.Name)
		items = append(items, Artifact{
			ID:          artifactID(ArtifactKindAgent, name),
			Kind:        ArtifactKindAgent,
			Name:        name,
			DisplayName: name,
			Description: cfg.Agent.Description,
			Source:      SourceLocal,
			SourceID:    "agent.config",
			Status:      StatusActive,
			Installed:   true,
			Bound:       true,
			Active:      true,
			Enabled:     true,
			Category:    "agent",
			Permissions: []string{cfg.Agent.PermissionLevel},
			TargetHints: []string{"main_agent"},
			Metadata: map[string]string{
				"working_dir": cfg.Agent.WorkingDir,
			},
		})
	}
	return items
}

func (c *LocalCatalog) agentStoreArtifacts() []Artifact {
	store := c.deps.AgentStore
	if store == nil {
		return nil
	}
	packages := store.List(agentstore.StoreFilter{})
	items := make([]Artifact, 0, len(packages))
	for _, pkg := range packages {
		id := strings.TrimSpace(pkg.ID)
		if id == "" {
			continue
		}
		installed := store.IsInstalled(id)
		status := StatusAvailable
		if installed {
			status = StatusInstalled
		}
		items = append(items, Artifact{
			ID:            artifactID(ArtifactKindAgent, id),
			Kind:          ArtifactKindAgent,
			Name:          firstNonEmpty(pkg.Name, id),
			DisplayName:   firstNonEmpty(pkg.DisplayName, pkg.Name, id),
			Description:   pkg.Description,
			Version:       pkg.Version,
			LatestVersion: pkg.Version,
			Source:        SourceLocal,
			SourceID:      "agentstore",
			Status:        status,
			Installed:     installed,
			Enabled:       true,
			Owner:         pkg.Author,
			Category:      firstNonEmpty(pkg.Category, pkg.Domain, "agent"),
			Tags:          appendUnique(pkg.Tags, pkg.Domain),
			Permissions:   []string{pkg.Permission},
			InstallHint:   "anyclaw store install " + id,
			TargetHints:   []string{"persistent_subagent", "workspace"},
			Capabilities:  appendUnique(appendUnique(pkg.Expertise, pkg.Skills...), pkg.Domain),
			Metadata: map[string]string{
				"store_id": id,
				"persona":  pkg.Persona,
			},
		})
	}
	return items
}

func (c *LocalCatalog) skillArtifacts() []Artifact {
	manager := c.deps.Skills
	if manager == nil {
		return nil
	}
	entries := manager.Catalog()
	items := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		status := StatusInstalled
		if !entry.Installed {
			status = StatusAvailable
		}
		items = append(items, Artifact{
			ID:            artifactID(ArtifactKindSkill, name),
			Kind:          ArtifactKindSkill,
			Name:          name,
			DisplayName:   firstNonEmpty(entry.FullName, name),
			Description:   entry.Description,
			Version:       entry.Version,
			LatestVersion: entry.Version,
			Source:        SourceLocal,
			SourceID:      firstNonEmpty(entry.Registry, entry.Source, "skills"),
			Status:        status,
			Installed:     entry.Installed,
			Enabled:       true,
			Category:      firstNonEmpty(entry.Category, "skill"),
			Tags:          appendUnique(nil, entry.Category, entry.Registry, entry.Source),
			Permissions:   append([]string(nil), entry.Permissions...),
			InstallHint:   entry.InstallHint,
			TargetHints:   []string{"main_agent", "persistent_subagent", "workspace"},
			Capabilities:  appendUnique(nil, entry.Category, name),
			Metadata: map[string]string{
				"homepage":      entry.Homepage,
				"entrypoint":    entry.Entrypoint,
				"installed_dir": entry.InstalledDir,
				"builtin":       boolString(entry.Builtin),
			},
		})
	}
	return items
}

func (c *LocalCatalog) cliArtifacts() []Artifact {
	catalog := c.deps.CLIHub
	if catalog == nil {
		return nil
	}
	entries := clihub.Search(catalog, "", "", false, 0)
	items := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		status := StatusAvailable
		installed := entry.Installed
		if entry.Runnable {
			status = StatusInstalled
		}
		items = append(items, Artifact{
			ID:            artifactID(ArtifactKindCLI, name),
			Kind:          ArtifactKindCLI,
			Name:          name,
			DisplayName:   firstNonEmpty(entry.DisplayName, name),
			Description:   entry.Description,
			Version:       entry.Version,
			LatestVersion: entry.Version,
			Source:        SourceLocal,
			SourceID:      "clihub",
			Status:        status,
			Installed:     installed,
			Enabled:       entry.Runnable,
			Owner:         entry.Contributor,
			Category:      firstNonEmpty(entry.Category, "cli"),
			Tags:          appendUnique(nil, entry.Category, entry.Requires),
			Permissions:   cliPermissions(entry),
			InstallHint:   entry.InstallCmd,
			TargetHints:   []string{"workspace", "runtime_global"},
			Capabilities:  appendUnique(nil, entry.Name, entry.DisplayName, entry.EntryPoint, entry.Category),
			Metadata: map[string]string{
				"entrypoint":      entry.EntryPoint,
				"homepage":        entry.Homepage,
				"requires":        entry.Requires,
				"run_mode":        entry.RunMode,
				"executable_path": entry.ExecutablePath,
				"source_path":     entry.SourcePath,
				"skill_path":      entry.SkillPath,
				"dev_module":      entry.DevModule,
				"catalog_root":    catalog.Root,
			},
		})
	}
	return items
}

func (c *LocalCatalog) pluginArtifacts() []Artifact {
	registry := c.deps.Plugins
	if registry == nil {
		return nil
	}
	manifests := registry.List()
	items := make([]Artifact, 0, len(manifests))
	for _, manifest := range manifests {
		name := strings.TrimSpace(manifest.Name)
		if name == "" {
			continue
		}
		status := StatusInstalled
		if !manifest.Enabled {
			status = StatusDisabled
		}
		items = append(items, Artifact{
			ID:            artifactID(ArtifactKindPlugin, name),
			Kind:          ArtifactKindPlugin,
			Name:          name,
			DisplayName:   name,
			Description:   manifest.Description,
			Version:       manifest.Version,
			LatestVersion: manifest.Version,
			Source:        SourceLocal,
			SourceID:      "plugins",
			Status:        status,
			Installed:     true,
			Enabled:       manifest.Enabled,
			Category:      firstNonEmpty(firstString(manifest.Kinds), "plugin"),
			Tags:          appendUnique(manifest.CapabilityTags, manifest.Kinds...),
			Permissions:   append([]string(nil), manifest.Permissions...),
			RiskLevel:     manifest.RiskLevel,
			TrustLevel:    manifest.Trust,
			Verified:      manifest.Verified,
			TargetHints:   []string{"runtime_global", "workspace"},
			Capabilities:  appendUnique(manifest.CapabilityTags, manifest.Kinds...),
			Metadata: map[string]string{
				"entrypoint":     manifest.Entrypoint,
				"approval_scope": manifest.ApprovalScope,
				"builtin":        boolString(manifest.Builtin),
			},
		})
	}
	return items
}

func filterArtifacts(items []Artifact, filter Filter) []Artifact {
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	out := make([]Artifact, 0, len(items))
	for _, item := range items {
		if filter.Kind != "" && item.Kind != filter.Kind {
			continue
		}
		if filter.Source != "" && item.Source != filter.Source {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.Risk != "" && !strings.EqualFold(item.RiskLevel, filter.Risk) {
			continue
		}
		if filter.Trust != "" && !strings.EqualFold(item.TrustLevel, filter.Trust) {
			continue
		}
		if filter.Tag != "" && !containsFold(item.Tags, filter.Tag) {
			continue
		}
		if filter.Permission != "" && !containsFold(item.Permissions, filter.Permission) {
			continue
		}
		if filter.Publisher != "" && !strings.Contains(strings.ToLower(item.Owner), strings.ToLower(strings.TrimSpace(filter.Publisher))) {
			continue
		}
		if filter.OS != "" && len(item.Compatibility.OS) > 0 && !containsFold(item.Compatibility.OS, filter.OS) {
			continue
		}
		if filter.Arch != "" && len(item.Compatibility.Arch) > 0 && !containsFold(item.Compatibility.Arch, filter.Arch) {
			continue
		}
		if query != "" && !artifactMatches(item, query) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func artifactMatches(item Artifact, query string) bool {
	parts := []string{
		item.ID,
		item.Name,
		item.DisplayName,
		item.Description,
		item.Version,
		item.Owner,
		item.Category,
		string(item.Kind),
		string(item.Source),
		item.SourceID,
		item.Status.String(),
	}
	parts = append(parts, item.Tags...)
	parts = append(parts, item.Permissions...)
	parts = append(parts, item.Capabilities...)
	for k, v := range item.Metadata {
		parts = append(parts, k, v)
	}
	return strings.Contains(strings.ToLower(strings.Join(parts, " ")), query)
}

func containsFold(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return true
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func sortArtifacts(items []Artifact) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

func dedupeArtifacts(items []Artifact) []Artifact {
	seen := make(map[string]bool, len(items))
	out := make([]Artifact, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		key := string(item.Source) + ":" + item.ID + ":" + item.SourceID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func (s ArtifactStatus) String() string {
	return string(s)
}

func artifactID(kind ArtifactKind, name string) string {
	return string(kind) + ":" + strings.TrimSpace(name)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstString(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUnique(base []string, values ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out)+len(values))
	filtered := make([]string, 0, len(out)+len(values))
	for _, value := range append(out, values...) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func agentPermissions(profile config.AgentProfile) []string {
	if strings.TrimSpace(profile.PermissionLevel) == "" {
		return nil
	}
	return []string{profile.PermissionLevel}
}

func agentCapabilities(profile config.AgentProfile) []string {
	values := appendUnique(profile.Expertise, profile.Domain, profile.Role)
	for _, skill := range profile.Skills {
		values = appendUnique(values, skill.Name)
	}
	return values
}

func cliPermissions(entry clihub.EntryStatus) []string {
	permissions := []string{"process.exec"}
	if strings.TrimSpace(entry.Homepage) != "" || strings.Contains(strings.ToLower(entry.InstallCmd), "http") {
		permissions = append(permissions, "network.http")
	}
	return permissions
}
