package auth

import "context"

type ContextKey string

const UserContextKey ContextKey = "auth-user"

type User struct {
	Name                string   `json:"name"`
	Role                string   `json:"role"`
	Permissions         []string `json:"permissions"`
	PermissionOverrides []string `json:"permission_overrides"`
	Scopes              []string `json:"scopes"`
	Orgs                []string `json:"orgs"`
	Projects            []string `json:"projects"`
	Workspaces          []string `json:"workspaces"`
}

func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(UserContextKey).(*User)
	return user
}

func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}
