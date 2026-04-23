package auth

import "github.com/1024XEngineer/anyclaw/pkg/config"

func BuiltinRoleTemplates() []config.SecurityRole {
	return []config.SecurityRole{
		{Name: "admin", Description: "Full platform access", Permissions: []string{"*"}},
		{Name: "operator", Description: "Operate sessions, runtimes, and workspace resources", Permissions: []string{"status.read", "chat.send", "tasks.read", "tasks.write", "approvals.read", "approvals.write", "sessions.read", "sessions.write", "memory.read", "runtimes.read", "runtimes.write", "events.read", "tools.read", "resources.read", "resources.write"}},
		{Name: "viewer", Description: "Read-only governance and monitoring", Permissions: []string{"status.read", "sessions.read", "events.read", "audit.read", "plugins.read", "channels.read", "routing.read", "runtimes.read", "resources.read"}},
	}
}

func resolveRolePermissions(cfg *config.SecurityConfig, roleName string) []string {
	if roleName == "admin" {
		return []string{"*"}
	}
	if cfg != nil {
		for _, role := range cfg.Roles {
			if role.Name == roleName {
				return append([]string{}, role.Permissions...)
			}
		}
	}
	for _, role := range BuiltinRoleTemplates() {
		if role.Name == roleName {
			return append([]string{}, role.Permissions...)
		}
	}
	switch roleName {
	case "viewer":
		return []string{"status.read", "sessions.read", "events.read", "audit.read", "plugins.read", "channels.read", "routing.read", "runtimes.read", "resources.read"}
	default:
		return nil
	}
}

func ResolveRolePermissions(cfg *config.SecurityConfig, roleName string) []string {
	return resolveRolePermissions(cfg, roleName)
}

func HasPermission(user *User, permission string) bool {
	if permission == "" {
		return true
	}
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	for _, granted := range user.Permissions {
		if granted == "*" || granted == permission {
			return true
		}
	}
	return false
}

func HasScope(user *User, workspace string) bool {
	if workspace == "" {
		return true
	}
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	if len(user.Scopes) == 0 {
		return false
	}
	for _, scope := range user.Scopes {
		if scope == "*" || scope == workspace {
			return true
		}
	}
	return false
}

func HasHierarchyAccess(user *User, org string, project string, workspace string) bool {
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	if workspace != "" {
		for _, item := range user.Workspaces {
			if item == "*" || item == workspace {
				return true
			}
		}
	}
	if project != "" {
		for _, item := range user.Projects {
			if item == "*" || item == project {
				return true
			}
		}
	}
	if org != "" {
		for _, item := range user.Orgs {
			if item == "*" || item == org {
				return true
			}
		}
		return false
	}
	return HasScope(user, workspace)
}
