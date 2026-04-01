package apps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

type Binding struct {
	ID          string            `json:"id"`
	App         string            `json:"app"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Org         string            `json:"org,omitempty"`
	Project     string            `json:"project,omitempty"`
	Workspace   string            `json:"workspace,omitempty"`
	Target      string            `json:"target,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	Secrets     map[string]string `json:"secrets,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type persistedState struct {
	Bindings []*Binding `json:"bindings"`
	Pairings []*Pairing `json:"pairings,omitempty"`
	Apps     []*AppInfo `json:"apps,omitempty"`
	UIMaps   []*UIMap   `json:"ui_maps,omitempty"`
	Updated  time.Time  `json:"updated"`
}

type Store struct {
	mu       sync.RWMutex
	path     string
	bindings []*Binding
	pairings []*Pairing
	apps     []*AppInfo
	uiMaps   []*UIMap
}

var bindingCounter uint64

func NewStore(configPath string) (*Store, error) {
	path, err := resolveStorePath(configPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	store := &Store{
		path:     path,
		bindings: []*Binding{},
		pairings: []*Pairing{},
		apps:     []*AppInfo{},
		uiMaps:   []*UIMap{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	s.bindings = cloneBindings(state.Bindings)
	s.pairings = clonePairings(state.Pairings)
	s.apps = state.Apps
	s.uiMaps = state.UIMaps
	return nil
}

func (s *Store) saveLocked() error {
	state := persistedState{
		Bindings: cloneBindings(s.bindings),
		Pairings: clonePairings(s.pairings),
		Apps:     s.apps,
		UIMaps:   s.uiMaps,
		Updated:  time.Now().UTC(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *Store) List() []*Binding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := cloneBindings(s.bindings)
	sort.Slice(items, func(i, j int) bool {
		if items[i].App == items[j].App {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return strings.ToLower(items[i].App) < strings.ToLower(items[j].App)
	})
	return items
}

func (s *Store) ListByApp(app string) []*Binding {
	app = normalizeToken(app)
	if app == "" {
		return s.List()
	}
	items := s.List()
	filtered := make([]*Binding, 0, len(items))
	for _, item := range items {
		if normalizeToken(item.App) == app {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (s *Store) Get(id string) (*Binding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	for _, binding := range s.bindings {
		if binding.ID == id {
			return cloneBinding(binding), true
		}
	}
	return nil, false
}

func (s *Store) Upsert(binding *Binding) error {
	if binding == nil {
		return fmt.Errorf("binding is required")
	}
	cleaned, err := normalizeBinding(binding)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for i, existing := range s.bindings {
		if existing.ID == cleaned.ID || sameBindingIdentity(existing, cleaned) {
			cleaned.ID = existing.ID
			cleaned.CreatedAt = existing.CreatedAt
			cleaned.UpdatedAt = now
			s.bindings[i] = cloneBinding(cleaned)
			return s.saveLocked()
		}
	}

	if cleaned.ID == "" {
		cleaned.ID = uniqueID("appbind")
	}
	cleaned.CreatedAt = now
	cleaned.UpdatedAt = now
	s.bindings = append(s.bindings, cloneBinding(cleaned))
	return s.saveLocked()
}

func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]*Binding, 0, len(s.bindings))
	removed := false
	for _, binding := range s.bindings {
		if binding.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, cloneBinding(binding))
	}
	if !removed {
		return fmt.Errorf("binding not found")
	}
	s.bindings = filtered
	return s.saveLocked()
}

func (s *Store) Resolve(app string, ref string) (*Binding, error) {
	app = normalizeToken(app)
	ref = strings.TrimSpace(ref)

	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]*Binding, 0)
	for _, binding := range s.bindings {
		if !binding.Enabled || normalizeToken(binding.App) != app {
			continue
		}
		candidates = append(candidates, binding)
	}

	if ref != "" {
		refLower := strings.ToLower(ref)
		for _, binding := range candidates {
			if strings.EqualFold(binding.ID, ref) || strings.EqualFold(binding.Name, refLower) || strings.EqualFold(binding.Name, ref) {
				return cloneBinding(binding), nil
			}
		}
		return nil, fmt.Errorf("app binding not found: %s", ref)
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) == 1 {
		return cloneBinding(candidates[0]), nil
	}
	return nil, fmt.Errorf("multiple bindings configured for app %s; specify binding", app)
}

func ResolveBindingEnvs(binding *Binding) []string {
	if binding == nil {
		return nil
	}
	configJSON, _ := json.Marshal(binding.Config)
	secretsJSON, _ := json.Marshal(binding.Secrets)
	metaJSON, _ := json.Marshal(binding.Metadata)
	env := []string{
		"ANYCLAW_APP_BINDING_ID=" + binding.ID,
		"ANYCLAW_APP_BINDING_APP=" + binding.App,
		"ANYCLAW_APP_BINDING_NAME=" + binding.Name,
		"ANYCLAW_APP_BINDING_TARGET=" + binding.Target,
		"ANYCLAW_APP_BINDING_ORG=" + binding.Org,
		"ANYCLAW_APP_BINDING_PROJECT=" + binding.Project,
		"ANYCLAW_APP_BINDING_WORKSPACE=" + binding.Workspace,
		"ANYCLAW_APP_BINDING_CONFIG_JSON=" + string(configJSON),
		"ANYCLAW_APP_BINDING_SECRETS_JSON=" + string(secretsJSON),
		"ANYCLAW_APP_BINDING_METADATA_JSON=" + string(metaJSON),
	}
	for key, value := range binding.Config {
		name := normalizeEnvKey(key)
		if name != "" {
			env = append(env, "ANYCLAW_APP_CONFIG_"+name+"="+value)
		}
	}
	for key, value := range binding.Secrets {
		name := normalizeEnvKey(key)
		if name != "" {
			env = append(env, "ANYCLAW_APP_SECRET_"+name+"="+value)
		}
	}
	return env
}

func resolveStorePath(configPath string) (string, error) {
	if strings.TrimSpace(configPath) == "" {
		configPath = "anyclaw.json"
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(absConfig), ".anyclaw", "apps", "bindings.json"), nil
}

func normalizeBinding(binding *Binding) (*Binding, error) {
	cleaned := cloneBinding(binding)
	cleaned.ID = strings.TrimSpace(cleaned.ID)
	cleaned.App = normalizeToken(cleaned.App)
	cleaned.Name = strings.TrimSpace(cleaned.Name)
	cleaned.Description = strings.TrimSpace(cleaned.Description)
	cleaned.Org = strings.TrimSpace(cleaned.Org)
	cleaned.Project = strings.TrimSpace(cleaned.Project)
	cleaned.Workspace = strings.TrimSpace(cleaned.Workspace)
	cleaned.Target = strings.TrimSpace(cleaned.Target)
	if cleaned.Config == nil {
		cleaned.Config = map[string]string{}
	}
	if cleaned.Secrets == nil {
		cleaned.Secrets = map[string]string{}
	}
	if cleaned.Metadata == nil {
		cleaned.Metadata = map[string]string{}
	}
	cleaned.Config = normalizeStringMap(cleaned.Config)
	cleaned.Secrets = normalizeStringMap(cleaned.Secrets)
	cleaned.Metadata = normalizeStringMap(cleaned.Metadata)
	if cleaned.App == "" {
		return nil, fmt.Errorf("app is required")
	}
	if cleaned.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !binding.Enabled && binding.ID == "" {
		cleaned.Enabled = true
	}
	return cleaned, nil
}

func sameBindingIdentity(a *Binding, b *Binding) bool {
	return normalizeToken(a.App) == normalizeToken(b.App) &&
		strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name)) &&
		strings.EqualFold(strings.TrimSpace(a.Org), strings.TrimSpace(b.Org)) &&
		strings.EqualFold(strings.TrimSpace(a.Project), strings.TrimSpace(b.Project)) &&
		strings.EqualFold(strings.TrimSpace(a.Workspace), strings.TrimSpace(b.Workspace))
}

func cloneBindings(items []*Binding) []*Binding {
	cloned := make([]*Binding, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, cloneBinding(item))
	}
	return cloned
}

func cloneBinding(binding *Binding) *Binding {
	if binding == nil {
		return nil
	}
	return &Binding{
		ID:          binding.ID,
		App:         binding.App,
		Name:        binding.Name,
		Description: binding.Description,
		Enabled:     binding.Enabled,
		Org:         binding.Org,
		Project:     binding.Project,
		Workspace:   binding.Workspace,
		Target:      binding.Target,
		Config:      cloneStringMap(binding.Config),
		Secrets:     cloneStringMap(binding.Secrets),
		Metadata:    cloneStringMap(binding.Metadata),
		CreatedAt:   binding.CreatedAt,
		UpdatedAt:   binding.UpdatedAt,
	}
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func normalizeStringMap(items map[string]string) map[string]string {
	cleaned := make(map[string]string)
	for key, value := range items {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" {
			cleaned[key] = value
		}
	}
	return cleaned
}

func normalizeToken(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func normalizeEnvKey(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func uniqueID(prefix string) string {
	seq := atomic.AddUint64(&bindingCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}

func (s *Store) ListApps() []*AppInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apps
}

func (s *Store) GetApp(id string) *AppInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, app := range s.apps {
		if app.ID == id {
			return app
		}
	}
	return nil
}

func (s *Store) UpsertApp(app *AppInfo) error {
	if app == nil {
		return fmt.Errorf("app is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.apps {
		if existing.ID == app.ID {
			app.UpdatedAt = time.Now()
			s.apps[i] = app
			return s.saveLocked()
		}
	}
	s.apps = append(s.apps, app)
	return s.saveLocked()
}

func (s *Store) DeleteApp(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]*AppInfo, 0)
	removed := false
	for _, app := range s.apps {
		if app.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, app)
	}
	if !removed {
		return fmt.Errorf("app not found")
	}
	s.apps = filtered
	return s.saveLocked()
}

func (s *Store) ListUIMaps() []*UIMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uiMaps
}

func (s *Store) GetUIMap(appID string) *UIMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, uiMap := range s.uiMaps {
		if uiMap.AppID == appID {
			return uiMap
		}
	}
	return nil
}

func (s *Store) UpsertUIMap(uiMap *UIMap) error {
	if uiMap == nil {
		return fmt.Errorf("uiMap is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.uiMaps {
		if existing.AppID == uiMap.AppID {
			s.uiMaps[i] = uiMap
			return s.saveLocked()
		}
	}
	s.uiMaps = append(s.uiMaps, uiMap)
	return s.saveLocked()
}
