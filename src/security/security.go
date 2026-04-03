package security

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Authenticator struct {
	mu         sync.RWMutex
	tokens     map[string]*Token
	users      map[string]*User
	sessionTTL time.Duration
}

type Token struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	Scopes    []string
}

type User struct {
	ID       string
	Username string
	Password string
	Roles    []string
}

func New() *Authenticator {
	return &Authenticator{
		tokens:     make(map[string]*Token),
		users:      make(map[string]*User),
		sessionTTL: 24 * time.Hour,
	}
}

func (a *Authenticator) Login(ctx context.Context, username, password string) (string, error) {
	a.mu.RLock()
	user, ok := a.users[username]
	a.mu.RUnlock()

	if !ok || user.Password != password {
		return "", fmt.Errorf("invalid credentials")
	}

	token := &Token{
		ID:        generateToken(),
		UserID:    user.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(a.sessionTTL),
		Scopes:    user.Roles,
	}

	a.mu.Lock()
	a.tokens[token.ID] = token
	a.mu.Unlock()

	return token.ID, nil
}

func (a *Authenticator) Validate(tokenID string) (*Token, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	token, ok := a.tokens[tokenID]
	if !ok {
		return nil, fmt.Errorf("token not found")
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, fmt.Errorf("token expired")
	}

	return token, nil
}

func (a *Authenticator) Logout(tokenID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, tokenID)
	return nil
}

func (a *Authenticator) Register(user *User) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.users[user.Username] = user
}

func generateToken() string {
	return fmt.Sprintf("%d-%d", time.Now().Unix(), time.Now().Nanosecond())
}

type Authorizer struct {
	mu       sync.RWMutex
	policies map[string][]Policy
}

type Policy struct {
	Resource string
	Action   string
	Roles    []string
}

func NewAuthorizer() *Authorizer {
	return &Authorizer{
		policies: make(map[string][]Policy),
	}
}

func (a *Authorizer) AddPolicy(policy Policy) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := fmt.Sprintf("%s:%s", policy.Resource, policy.Action)
	a.policies[key] = append(a.policies[key], policy)
}

func (a *Authorizer) Can(role, resource, action string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", resource, action)
	policies, ok := a.policies[key]
	if !ok {
		return false
	}

	for _, p := range policies {
		for _, r := range p.Roles {
			if r == role || r == "*" {
				return true
			}
		}
	}
	return false
}
