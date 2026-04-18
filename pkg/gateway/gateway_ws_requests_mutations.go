package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (c *openClawWSConn) handleMutationWSRequest(ctx context.Context, frame openClawWSFrame, method string) (bool, error) {
	switch method {
	case "providers.update":
		if err := c.requirePermission("config.write"); err != nil {
			return true, err
		}
		providerJSON, err := marshalWSParam(frame.Params, "provider", "provider data required", "invalid provider data")
		if err != nil {
			return true, err
		}
		var provider config.ProviderProfile
		if err := json.Unmarshal(providerJSON, &provider); err != nil {
			return true, fmt.Errorf("invalid provider format: %v", err)
		}
		var result providerView
		if err := c.invokeWSJSONPost(ctx, "/api/providers", providerJSON, c.server.handleProviders, &result, "provider update failed"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, result, "")
	case "providers.default":
		if err := c.requirePermission("config.write"); err != nil {
			return true, err
		}
		providerRef := mapString(frame.Params, "provider_ref")
		if providerRef == "" {
			return true, fmt.Errorf("provider_ref required")
		}
		reqBody, err := json.Marshal(map[string]string{"provider_ref": providerRef})
		if err != nil {
			return true, fmt.Errorf("failed to marshal request: %v", err)
		}
		var result providerView
		if err := c.invokeWSJSONPost(ctx, "/api/providers/default", reqBody, c.server.handleDefaultProvider, &result, "default provider update failed"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, result, "")
	case "providers.test":
		if err := c.requireConfigRead(); err != nil {
			return true, err
		}
		providerJSON, err := marshalWSParam(frame.Params, "provider", "provider data required", "invalid provider data")
		if err != nil {
			return true, err
		}
		var result providerHealth
		if err := c.invokeWSJSONPost(ctx, "/api/providers/test", providerJSON, c.server.handleProviderTest, &result, "provider test failed"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, result, "")
	case "agent-bindings.update", "agent_bindings.update":
		if err := c.requirePermission("config.write"); err != nil {
			return true, err
		}
		bindingJSON, err := marshalWSParam(frame.Params, "binding", "binding data required", "invalid binding data")
		if err != nil {
			return true, err
		}
		var result []agentBindingView
		if err := c.invokeWSJSONPost(ctx, "/api/agent-bindings", bindingJSON, c.server.handleAgentBindings, &result, "agent binding update failed"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, result, "")
	default:
		return false, nil
	}
}

func marshalWSParam(params map[string]any, key string, requiredMsg string, invalidMsg string) ([]byte, error) {
	if params == nil {
		return nil, fmt.Errorf("params required")
	}
	value := params[key]
	if value == nil {
		return nil, fmt.Errorf("%s", requiredMsg)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", invalidMsg, err)
	}
	return body, nil
}

func (c *openClawWSConn) invokeWSJSONPost(ctx context.Context, path string, body []byte, handler func(http.ResponseWriter, *http.Request), target any, failurePrefix string) error {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req = req.WithContext(ctx)
	handler(recorder, req)
	if recorder.Code >= 400 {
		return fmt.Errorf("%s: %s", failurePrefix, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}
	return nil
}
