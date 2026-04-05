package enterprise

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// PermissionResolver handles fine-grained RBAC with role inheritance,
// wildcard patterns, and attribute-based access control (ABAC).
type PermissionResolver struct {
	mu            sync.RWMutex
	roles         map[string]*Role
	users         map[string]*UserPermissions
	policies      []ABACPolicy
	resourcePerms map[string]*ResourcePermissions
}

// Role defines a role with inheritance and permissions.
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Inherits    []string `json:"inherits,omitempty"`
	Conditions  []string `json:"conditions,omitempty"`
}

// UserPermissions holds per-user permission overrides and attributes.
type UserPermissions struct {
	UserID              string            `json:"user_id"`
	Roles               []string          `json:"roles"`
	PermissionOverrides map[string]bool   `json:"permission_overrides"` // true=grant, false=deny
	Attributes          map[string]string `json:"attributes"`
	DeniedPermissions   []string          `json:"denied_permissions"`
}

// ABACPolicy is an attribute-based access control policy.
type ABACPolicy struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Effect      string            `json:"effect"`   // "allow" or "deny"
	Subject     map[string]string `json:"subject"`  // attribute matchers
	Resource    map[string]string `json:"resource"` // resource attribute matchers
	Action      string            `json:"action"`
	Condition   string            `json:"condition,omitempty"` // condition expression
}

// ResourcePermissions defines permissions for a specific resource.
type ResourcePermissions struct {
	ResourceType string              `json:"resource_type"`
	ResourceID   string              `json:"resource_id"`
	Permissions  map[string][]string `json:"permissions"` // role -> []permissions
}

// NewPermissionResolver creates a new RBAC resolver.
func NewPermissionResolver() *PermissionResolver {
	return &PermissionResolver{
		roles:         make(map[string]*Role),
		users:         make(map[string]*UserPermissions),
		policies:      make([]ABACPolicy, 0),
		resourcePerms: make(map[string]*ResourcePermissions),
	}
}

// AddRole adds or updates a role.
func (pr *PermissionResolver) AddRole(role Role) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.roles[role.Name] = &role
}

// DeleteRole removes a role.
func (pr *PermissionResolver) DeleteRole(name string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	delete(pr.roles, name)
}

// GetRole returns a role by name.
func (pr *PermissionResolver) GetRole(name string) (*Role, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	role, ok := pr.roles[name]
	return role, ok
}

// ListRoles returns all role names.
func (pr *PermissionResolver) ListRoles() []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	names := make([]string, 0, len(pr.roles))
	for name := range pr.roles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetUserPermissions sets permissions for a user.
func (pr *PermissionResolver) SetUserPermissions(up UserPermissions) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if up.PermissionOverrides == nil {
		up.PermissionOverrides = make(map[string]bool)
	}
	if up.Attributes == nil {
		up.Attributes = make(map[string]string)
	}
	pr.users[up.UserID] = &up
}

// GetUserPermissions returns permissions for a user.
func (pr *PermissionResolver) GetUserPermissions(userID string) (*UserPermissions, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	up, ok := pr.users[userID]
	return up, ok
}

// AddABACPolicy adds an attribute-based access control policy.
func (pr *PermissionResolver) AddABACPolicy(policy ABACPolicy) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.policies = append(pr.policies, policy)
}

// SetResourcePermissions sets permissions for a specific resource.
func (pr *PermissionResolver) SetResourcePermissions(rp ResourcePermissions) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	key := rp.ResourceType + ":" + rp.ResourceID
	if rp.Permissions == nil {
		rp.Permissions = make(map[string][]string)
	}
	pr.resourcePerms[key] = &rp
}

// ResolvePermissions returns the full set of permissions for a user,
// including inherited roles, overrides, and ABAC policies.
func (pr *PermissionResolver) ResolvePermissions(userID string) ([]string, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	up, ok := pr.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	perms := make(map[string]bool)

	// Collect permissions from all roles (with inheritance)
	for _, roleName := range up.Roles {
		rolePerms := pr.resolveRolePermissions(roleName, make(map[string]bool))
		for p := range rolePerms {
			perms[p] = true
		}
	}

	// Apply explicit overrides
	for perm, granted := range up.PermissionOverrides {
		perms[perm] = granted
	}

	// Apply explicit denials
	for _, denied := range up.DeniedPermissions {
		if matchWildcard(denied, perms) {
			// Remove all matching permissions
			for p := range perms {
				if matchPattern(denied, p) {
					delete(perms, p)
				}
			}
		} else {
			delete(perms, denied)
		}
	}

	// Evaluate ABAC policies (deny takes precedence)
	for _, policy := range pr.policies {
		if pr.matchABACPolicy(policy, up) {
			if policy.Effect == "deny" {
				delete(perms, policy.Action)
			} else {
				perms[policy.Action] = true
			}
		}
	}

	result := make([]string, 0, len(perms))
	for p := range perms {
		if perms[p] {
			result = append(result, p)
		}
	}
	sort.Strings(result)
	return result, nil
}

// HasPermission checks if a user has a specific permission.
func (pr *PermissionResolver) HasPermission(userID, permission string) (bool, error) {
	perms, err := pr.ResolvePermissions(userID)
	if err != nil {
		return false, err
	}

	for _, p := range perms {
		if p == "*" {
			return true, nil
		}
		if matchPattern(p, permission) {
			return true, nil
		}
	}
	return false, nil
}

// HasResourcePermission checks if a user has permission on a specific resource.
func (pr *PermissionResolver) HasResourcePermission(userID, resourceType, resourceID, permission string) (bool, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	up, ok := pr.users[userID]
	if !ok {
		return false, fmt.Errorf("user not found: %s", userID)
	}

	key := resourceType + ":" + resourceID
	if rp, exists := pr.resourcePerms[key]; exists {
		for _, roleName := range up.Roles {
			if rolePerms, ok := rp.Permissions[roleName]; ok {
				for _, p := range rolePerms {
					if p == "*" || p == permission {
						return true, nil
					}
				}
			}
		}
	}

	// Fall back to global permissions
	pr.mu.RUnlock()
	return pr.HasPermission(userID, permission)
}

// resolveRolePermissions recursively resolves a role's permissions including inherited roles.
func (pr *PermissionResolver) resolveRolePermissions(roleName string, visited map[string]bool) map[string]bool {
	if visited[roleName] {
		return nil // prevent circular inheritance
	}
	visited[roleName] = true

	role, ok := pr.roles[roleName]
	if !ok {
		return nil
	}

	perms := make(map[string]bool)

	// Inherit from parent roles
	for _, parent := range role.Inherits {
		parentPerms := pr.resolveRolePermissions(parent, visited)
		for p := range parentPerms {
			perms[p] = true
		}
	}

	// Add own permissions
	for _, p := range role.Permissions {
		perms[p] = true
	}

	return perms
}

// matchABACPolicy checks if an ABAC policy applies to a user.
func (pr *PermissionResolver) matchABACPolicy(policy ABACPolicy, up *UserPermissions) bool {
	for attrKey, attrVal := range policy.Subject {
		userVal, ok := up.Attributes[attrKey]
		if !ok || userVal != attrVal {
			return false
		}
	}
	return true
}

// matchWildcard checks if a permission string contains wildcard patterns.
func matchWildcard(pattern string, perms map[string]bool) bool {
	return strings.Contains(pattern, "*")
}

// matchPattern checks if a permission matches a pattern (supports wildcards).
func matchPattern(pattern, permission string) bool {
	if pattern == permission {
		return true
	}
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}

	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		if parts[0] == "" && parts[1] == "" {
			return true
		}
		if parts[0] == "" {
			return strings.HasSuffix(permission, parts[1])
		}
		if parts[1] == "" {
			return strings.HasPrefix(permission, parts[0])
		}
		return strings.HasPrefix(permission, parts[0]) && strings.HasSuffix(permission, parts[1])
	}

	return false
}

// GetRoleGraph returns the role inheritance graph.
func (pr *PermissionResolver) GetRoleGraph() map[string][]string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	graph := make(map[string][]string)
	for name, role := range pr.roles {
		graph[name] = append([]string{}, role.Inherits...)
	}
	return graph
}

// GetEffectivePermissions returns the effective permissions for all roles
// (useful for debugging and admin UI).
func (pr *PermissionResolver) GetEffectivePermissions() map[string][]string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	result := make(map[string][]string)
	for name := range pr.roles {
		perms := pr.resolveRolePermissions(name, make(map[string]bool))
		list := make([]string, 0, len(perms))
		for p := range perms {
			list = append(list, p)
		}
		sort.Strings(list)
		result[name] = list
	}
	return result
}
