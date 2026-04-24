package intake

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

type PluginIngressAPI struct {
	IngressPlugins []plugin.IngressRunner
	CurrentUser    CurrentUserFunc
	AppendAudit    AuditRecorder
	AppendEvent    EventRecorder
}

func (api PluginIngressAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pluginName := strings.TrimPrefix(r.URL.Path, "/ingress/plugins/")
	if pluginName == "" {
		http.NotFound(w, r)
		return
	}
	runner, ok := api.findIngressRunner(pluginName)
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	output, err := executeIngressRunner(r.Context(), runner, body)
	if err != nil {
		writeJSON(w, ingressRunnerErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	api.appendEvent("ingress.plugin.accepted", "", map[string]any{"plugin": runner.Manifest.Name})
	api.appendAudit(api.currentUser(r.Context()), "ingress.plugin.accepted", runner.Manifest.Name, nil)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(output)
}

func (api PluginIngressAPI) findIngressRunner(pluginName string) (*plugin.IngressRunner, bool) {
	for i := range api.IngressPlugins {
		if api.IngressPlugins[i].Manifest.Name == pluginName {
			return &api.IngressPlugins[i], true
		}
	}
	return nil, false
}

func executeIngressRunner(ctx context.Context, runner *plugin.IngressRunner, body []byte) ([]byte, error) {
	if runner == nil {
		return nil, fmt.Errorf("plugin ingress not found")
	}
	runCtx, cancel := context.WithTimeout(ctx, runner.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runner.Entrypoint)
	pluginDir := filepath.Dir(runner.Entrypoint)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(),
		"ANYCLAW_PLUGIN_INPUT="+string(body),
		"ANYCLAW_PLUGIN_DIR="+pluginDir,
		"ANYCLAW_PLUGIN_TIMEOUT_SECONDS="+fmt.Sprintf("%d", int(runner.Timeout/time.Second)),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("plugin ingress timed out")
		}
		return nil, fmt.Errorf("plugin ingress failed: %s", string(output))
	}
	return output, nil
}

func ingressRunnerErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if strings.Contains(err.Error(), "timed out") {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func (api PluginIngressAPI) currentUser(ctx context.Context) *gatewayauth.User {
	if api.CurrentUser == nil {
		return nil
	}
	return api.CurrentUser(ctx)
}

func (api PluginIngressAPI) appendAudit(user *gatewayauth.User, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(user, action, target, meta)
}

func (api PluginIngressAPI) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if api.AppendEvent == nil {
		return
	}
	api.AppendEvent(eventType, sessionID, payload)
}
