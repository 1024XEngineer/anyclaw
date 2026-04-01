package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func gatewayHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func gatewayURL(cfg *config.Config, path string) string {
	baseURL := strings.TrimRight(appRuntime.GatewayURL(cfg), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func newGatewayRequest(ctx context.Context, cfg *config.Config, method string, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, gatewayURL(cfg, path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if cfg != nil {
		token := strings.TrimSpace(cfg.Security.APIToken)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func doGatewayJSONRequest(ctx context.Context, cfg *config.Config, method string, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := newGatewayRequest(ctx, cfg, method, path, body)
	if err != nil {
		return err
	}

	resp, err := gatewayHTTPClient(5 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		if len(payload) == 0 {
			return fmt.Errorf("gateway returned %s", resp.Status)
		}
		return fmt.Errorf("gateway returned %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	if responseBody == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(responseBody)
}
