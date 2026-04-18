package intake

import (
	"context"
	"fmt"
	"net/http"

	webhookext "github.com/1024XEngineer/anyclaw/pkg/extensions/adapters/webhook"
	runtimepkg "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

type RuntimePoolProvider interface {
	GetOrCreate(agentName string, org string, project string, workspaceID string) (*runtimepkg.MainRuntime, error)
}

type WebhookIngressAPI struct {
	Webhooks               *webhookext.Handler
	MainRuntime            *runtimepkg.MainRuntime
	RuntimePool            RuntimePoolProvider
	EnsureDefaultWorkspace func() error
	DefaultResourceIDs     func(workingDir string) (string, string, string)
	AppendEvent            EventRecorder
}

func (api WebhookIngressAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Webhooks == nil {
		http.NotFound(w, r)
		return
	}
	statusCode, body := api.Webhooks.HandleRequest(r.Context(), r, func(ctx context.Context, webhook *webhookext.Webhook, payload []byte) (string, error) {
		agentName := webhook.Agent
		if agentName == "" && api.MainRuntime != nil && api.MainRuntime.Config != nil {
			agentName = api.MainRuntime.Config.ResolveMainAgentName()
		}
		if api.EnsureDefaultWorkspace != nil {
			if err := api.EnsureDefaultWorkspace(); err != nil {
				return "", err
			}
		}
		if api.MainRuntime == nil || api.RuntimePool == nil || api.DefaultResourceIDs == nil {
			return "", fmt.Errorf("webhook ingress runtime not configured")
		}
		orgID, projectID, workspaceID := api.DefaultResourceIDs(api.MainRuntime.WorkingDir)
		targetRuntime, err := api.RuntimePool.GetOrCreate(agentName, orgID, projectID, workspaceID)
		if err != nil {
			return "", err
		}

		message := fmt.Sprintf("[Webhook: %s] %s", webhook.Name, string(payload))
		if webhook.Template != "" {
			message = fmt.Sprintf("%s\n\nPayload:\n%s", webhook.Template, string(payload))
		}

		execResult, err := targetRuntime.Execute(ctx, runtimepkg.ExecutionRequest{
			Input:          message,
			ReplaceHistory: true,
		})
		response := ""
		if execResult != nil {
			response = execResult.Output
		}
		if err != nil {
			return "", err
		}

		api.appendEvent("webhook.triggered", "", map[string]any{
			"webhook_id": webhook.ID,
			"name":       webhook.Name,
			"response":   response,
		})
		return response, nil
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

func (api WebhookIngressAPI) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if api.AppendEvent == nil {
		return
	}
	api.AppendEvent(eventType, sessionID, payload)
}
