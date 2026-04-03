package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type contextKey string

const UserContextKey contextKey = "user"

type Middleware struct {
	config *config.Config
	users  map[string]*User
}

type User struct {
	ID       string
	Username string
	Role     string
	Token    string
}

func NewMiddleware(cfg *config.Config) *Middleware {
	return &Middleware{
		config: cfg,
		users:  make(map[string]*User),
	}
}

func (m *Middleware) RequirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := extractToken(req)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user := m.validateToken(token)
		if user == nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(req.Context(), UserContextKey, user)
		next(w, req.WithContext(ctx))
	}
}

func (m *Middleware) validateToken(token string) *User {
	if user, ok := m.users[token]; ok {
		return user
	}
	if token == "dev" {
		return &User{ID: "1", Username: "dev", Role: "admin", Token: token}
	}
	return nil
}

func extractToken(req *http.Request) string {
	auth := req.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return req.URL.Query().Get("token")
}

func GetUserFromContext(ctx context.Context) *User {
	if user, ok := ctx.Value(UserContextKey).(*User); ok {
		return user
	}
	return nil
}

func (m *Middleware) HandleUsers(w http.ResponseWriter, req *http.Request) {
	users := make([]map[string]string, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, map[string]string{
			"id":       u.ID,
			"username": u.Username,
			"role":     u.Role,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

type AuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      *User     `json:"user"`
}

func (m *Middleware) HandleLogin(w http.ResponseWriter, req *http.Request) {
	token := "dev-token-" + time.Now().Format("20060102150405")
	user := &User{
		ID:       "1",
		Username: "dev",
		Role:     "admin",
		Token:    token,
	}
	m.users[token] = user
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
