package runtime

import (
	"path/filepath"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestResolveConfigPathUsesAbsolutePath(t *testing.T) {
	relative := filepath.Join("testdata", "anyclaw.json")
	got := ResolveConfigPath(relative)
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute config path, got %q", got)
	}

	abs := filepath.Join(t.TempDir(), "anyclaw.json")
	if got := ResolveConfigPath(abs); got != abs {
		t.Fatalf("expected absolute path to stay unchanged, got %q", got)
	}
}

func TestSanitizeTargetName(t *testing.T) {
	tests := map[string]string{
		"":                      "default",
		"  ":                    "default",
		" My Target.Name ":      "my-target.name",
		"..bad///name..":        "bad-name",
		"HELLO_world-01":        "hello_world-01",
		"!!!":                   "default",
		"multi   part   target": "multi-part-target",
	}

	for input, want := range tests {
		if got := sanitizeTargetName(input); got != want {
			t.Fatalf("sanitizeTargetName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGatewayAddressAndURLDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = ""
	cfg.Gateway.Port = 0

	if got := GatewayAddress(cfg); got != "127.0.0.1:18789" {
		t.Fatalf("expected default gateway address, got %q", got)
	}
	if got := GatewayURL(cfg); got != "ws://127.0.0.1:18789/ws" {
		t.Fatalf("expected default gateway URL, got %q", got)
	}
}

func TestGatewayAddressAndURLCustomValues(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 19090

	if got := GatewayAddress(cfg); got != "0.0.0.0:19090" {
		t.Fatalf("expected custom gateway address, got %q", got)
	}
	if got := GatewayURL(cfg); got != "ws://0.0.0.0:19090/ws" {
		t.Fatalf("expected custom gateway URL, got %q", got)
	}
}
