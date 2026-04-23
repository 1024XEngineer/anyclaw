package governance

import (
	"context"
	"strings"
	"testing"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

func TestAuthorizeConnectionRequiresAuthenticatedCaller(t *testing.T) {
	service := Service{}

	_, err := service.AuthorizeConnection(context.Background(), ConnectionRequest{
		ConnectionID: "ws-1",
		Protocol:     "ws",
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}

func TestAuthorizeConnectionAcceptsAuthenticatedCaller(t *testing.T) {
	user := &gatewayauth.User{Name: "operator", Role: "operator", Permissions: []string{"status.read"}}
	service := Service{CurrentUser: func(context.Context) *gatewayauth.User { return user }}

	authz, err := service.AuthorizeConnection(context.Background(), ConnectionRequest{
		ConnectionID: "ws-1",
		Protocol:     "ws",
		ClientID:     "client-1",
	})
	if err != nil {
		t.Fatalf("AuthorizeConnection returned error: %v", err)
	}
	if !authz.Result.Authenticated {
		t.Fatal("expected authenticated result")
	}
	if authz.Caller.UserID != "operator" {
		t.Fatalf("expected operator caller, got %q", authz.Caller.UserID)
	}
}

func TestAuthorizeCommandChecksPermission(t *testing.T) {
	user := &gatewayauth.User{Name: "viewer", Role: "viewer", Permissions: []string{"status.read"}}
	service := Service{CurrentUser: func(context.Context) *gatewayauth.User { return user }}

	if _, err := service.AuthorizeCommand(context.Background(), CommandRequest{
		Method:             "status.get",
		RequiredPermission: "status.read",
	}); err != nil {
		t.Fatalf("AuthorizeCommand returned error: %v", err)
	}

	_, err := service.AuthorizeCommand(context.Background(), CommandRequest{
		Method:             "chat.send",
		RequiredPermission: "chat.send",
	})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
}

func TestAuthorizeCommandAllowsAdmin(t *testing.T) {
	user := &gatewayauth.User{Name: "admin", Role: "admin", Permissions: []string{"*"}}
	service := Service{CurrentUser: func(context.Context) *gatewayauth.User { return user }}

	_, err := service.AuthorizeCommand(context.Background(), CommandRequest{
		Method:             "config.write",
		RequiredPermission: "config.write",
	})
	if err != nil {
		t.Fatalf("expected admin command authorization, got %v", err)
	}
}
