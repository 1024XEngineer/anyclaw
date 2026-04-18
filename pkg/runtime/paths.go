package runtime

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	runtimebootstrap "github.com/1024XEngineer/anyclaw/pkg/runtime/bootstrap"
)

func resolveRuntimePaths(cfg *config.Config, configPath string) {
	runtimebootstrap.ResolveRuntimePaths(cfg, configPath)
}

func ResolveConfigPath(path string) string {
	return runtimebootstrap.ResolveConfigPath(path)
}

func sanitizeTargetName(input string) string {
	clean := strings.TrimSpace(strings.ToLower(input))
	if clean == "" {
		return "default"
	}
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	clean = re.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-.")
	if clean == "" {
		return "default"
	}
	return clean
}

func GatewayAddress(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Gateway.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Gateway.Port
	if port <= 0 {
		port = 18789
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func GatewayURL(cfg *config.Config) string {
	return "ws://" + GatewayAddress(cfg) + "/ws"
}
