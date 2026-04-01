package apps

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Pairing struct {
	ID          string            `json:"id"`
	App         string            `json:"app"`
	Workflow    string            `json:"workflow,omitempty"`
	Binding     string            `json:"binding,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Org         string            `json:"org,omitempty"`
	Project     string            `json:"project,omitempty"`
	Workspace   string            `json:"workspace,omitempty"`
	Triggers    []string          `json:"triggers,omitempty"`
	Defaults    map[string]any    `json:"defaults,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func (s *Store) ListPairings() []*Pairing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := clonePairings(s.pairings)
	sort.Slice(items, func(i, j int) bool {
		if items[i].App == items[j].App {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return strings.ToLower(items[i].App) < strings.ToLower(items[j].App)
	})
	return items
}

func (s *Store) ListPairingsByApp(app string) []*Pairing {
	app = normalizeToken(app)
	if app == "" {
		return s.ListPairings()
	}
	items := s.ListPairings()
	filtered := make([]*Pairing, 0, len(items))
	for _, item := range items {
		if normalizeToken(item.App) == app {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (s *Store) GetPairing(id string) (*Pairing, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	for _, pairing := range s.pairings {
		if pairing.ID == id {
			return clonePairing(pairing), true
		}
	}
	return nil, false
}

func (s *Store) UpsertPairing(pairing *Pairing) error {
	if pairing == nil {
		return fmt.Errorf("pairing is required")
	}
	cleaned, err := normalizePairing(pairing)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for i, existing := range s.pairings {
		if existing.ID == cleaned.ID || samePairingIdentity(existing, cleaned) {
			cleaned.ID = existing.ID
			cleaned.CreatedAt = existing.CreatedAt
			cleaned.UpdatedAt = now
			s.pairings[i] = clonePairing(cleaned)
			return s.saveLocked()
		}
	}

	if cleaned.ID == "" {
		cleaned.ID = uniqueID("apppair")
	}
	cleaned.CreatedAt = now
	cleaned.UpdatedAt = now
	s.pairings = append(s.pairings, clonePairing(cleaned))
	return s.saveLocked()
}

func (s *Store) DeletePairing(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]*Pairing, 0, len(s.pairings))
	removed := false
	for _, pairing := range s.pairings {
		if pairing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, clonePairing(pairing))
	}
	if !removed {
		return fmt.Errorf("pairing not found")
	}
	s.pairings = filtered
	return s.saveLocked()
}

func (s *Store) ResolvePairing(app string, ref string) (*Pairing, error) {
	app = normalizeToken(app)
	ref = strings.TrimSpace(ref)

	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]*Pairing, 0)
	for _, pairing := range s.pairings {
		if !pairing.Enabled {
			continue
		}
		if app != "" && normalizeToken(pairing.App) != app {
			continue
		}
		candidates = append(candidates, pairing)
	}

	if ref != "" {
		for _, pairing := range candidates {
			if strings.EqualFold(pairing.ID, ref) || strings.EqualFold(pairing.Name, ref) {
				return clonePairing(pairing), nil
			}
		}
		return nil, fmt.Errorf("app pairing not found: %s", ref)
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) == 1 {
		return clonePairing(candidates[0]), nil
	}
	return nil, fmt.Errorf("multiple pairings configured for app %s; specify pairing", app)
}

func normalizePairing(pairing *Pairing) (*Pairing, error) {
	cleaned := clonePairing(pairing)
	cleaned.ID = strings.TrimSpace(cleaned.ID)
	cleaned.App = normalizeToken(cleaned.App)
	cleaned.Workflow = strings.TrimSpace(cleaned.Workflow)
	cleaned.Binding = strings.TrimSpace(cleaned.Binding)
	cleaned.Name = strings.TrimSpace(cleaned.Name)
	cleaned.Description = strings.TrimSpace(cleaned.Description)
	cleaned.Org = strings.TrimSpace(cleaned.Org)
	cleaned.Project = strings.TrimSpace(cleaned.Project)
	cleaned.Workspace = strings.TrimSpace(cleaned.Workspace)
	cleaned.Triggers = normalizeStringSlice(cleaned.Triggers)
	if cleaned.Defaults == nil {
		cleaned.Defaults = map[string]any{}
	}
	if cleaned.Metadata == nil {
		cleaned.Metadata = map[string]string{}
	}
	cleaned.Defaults = normalizeAnyMap(cleaned.Defaults)
	cleaned.Metadata = normalizeStringMap(cleaned.Metadata)
	if cleaned.App == "" {
		return nil, fmt.Errorf("app is required")
	}
	if cleaned.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if cleaned.Workflow == "" {
		return nil, fmt.Errorf("workflow is required")
	}
	if !pairing.Enabled && pairing.ID == "" {
		cleaned.Enabled = true
	}
	return cleaned, nil
}

func samePairingIdentity(a *Pairing, b *Pairing) bool {
	return normalizeToken(a.App) == normalizeToken(b.App) &&
		strings.EqualFold(strings.TrimSpace(a.Workflow), strings.TrimSpace(b.Workflow)) &&
		strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name)) &&
		strings.EqualFold(strings.TrimSpace(a.Org), strings.TrimSpace(b.Org)) &&
		strings.EqualFold(strings.TrimSpace(a.Project), strings.TrimSpace(b.Project)) &&
		strings.EqualFold(strings.TrimSpace(a.Workspace), strings.TrimSpace(b.Workspace))
}

func clonePairings(items []*Pairing) []*Pairing {
	cloned := make([]*Pairing, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, clonePairing(item))
	}
	return cloned
}

func clonePairing(pairing *Pairing) *Pairing {
	if pairing == nil {
		return nil
	}
	return &Pairing{
		ID:          pairing.ID,
		App:         pairing.App,
		Workflow:    pairing.Workflow,
		Binding:     pairing.Binding,
		Name:        pairing.Name,
		Description: pairing.Description,
		Enabled:     pairing.Enabled,
		Org:         pairing.Org,
		Project:     pairing.Project,
		Workspace:   pairing.Workspace,
		Triggers:    append([]string{}, pairing.Triggers...),
		Defaults:    cloneAnyMap(pairing.Defaults),
		Metadata:    cloneStringMap(pairing.Metadata),
		CreatedAt:   pairing.CreatedAt,
		UpdatedAt:   pairing.UpdatedAt,
	}
}

func cloneAnyMap(items map[string]any) map[string]any {
	if len(items) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(items))
	for key, value := range items {
		switch typed := value.(type) {
		case map[string]any:
			cloned[key] = cloneAnyMap(typed)
		case []any:
			cloned[key] = cloneAnySlice(typed)
		default:
			cloned[key] = typed
		}
	}
	return cloned
}

func cloneAnySlice(items []any) []any {
	if len(items) == 0 {
		return []any{}
	}
	cloned := make([]any, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]any:
			cloned = append(cloned, cloneAnyMap(typed))
		case []any:
			cloned = append(cloned, cloneAnySlice(typed))
		default:
			cloned = append(cloned, typed)
		}
	}
	return cloned
}

func normalizeAnyMap(items map[string]any) map[string]any {
	cleaned := make(map[string]any, len(items))
	for key, value := range items {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		switch typed := value.(type) {
		case string:
			cleaned[key] = strings.TrimSpace(typed)
		case map[string]any:
			cleaned[key] = normalizeAnyMap(typed)
		case []any:
			cleaned[key] = cloneAnySlice(typed)
		default:
			cleaned[key] = typed
		}
	}
	return cleaned
}

func normalizeStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		cleaned = append(cleaned, item)
	}
	return cleaned
}
