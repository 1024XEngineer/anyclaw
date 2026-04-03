// Package llm provides LLM types and functions.
// It re-exports from the internal providers package.
package llm

import (
	llm "github.com/anyclaw/anyclaw/pkg/providers"
)

type Config = llm.Config
type Client = llm.Client
type Message = llm.Message
type Response = llm.Response
type ToolDefinition = llm.ToolDefinition
type ToolFunctionDefinition = llm.ToolFunctionDefinition
type Usage = llm.Usage
type ToolCall = llm.ToolCall
type FunctionCall = llm.FunctionCall
type ClientWrapper = llm.ClientWrapper

func NewClient(cfg Config) (Client, error) {
	return llm.NewClient(cfg)
}

func NewClientWrapper(cfg Config) (*ClientWrapper, error) {
	return llm.NewClientWrapper(cfg)
}

func NewClientWrapperString(provider, model, apiKey, baseURL string) (*ClientWrapper, error) {
	cfg := Config{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}
	return llm.NewClientWrapper(cfg)
}

func NormalizeProviderName(provider string) string {
	return llm.NormalizeProviderName(provider)
}

func ProviderRequiresAPIKey(provider string) bool {
	return llm.ProviderRequiresAPIKey(provider)
}
