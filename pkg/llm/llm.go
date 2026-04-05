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

type ContentType = llm.ContentType
type ContentBlock = llm.ContentBlock
type ImageURLBlock = llm.ImageURLBlock
type ImageBlock = llm.ImageBlock
type ImageSource = llm.ImageSource

const (
	ContentTypeText     = llm.ContentTypeText
	ContentTypeImageURL = llm.ContentTypeImageURL
	ContentTypeImage    = llm.ContentTypeImage
	ContentTypeFile     = llm.ContentTypeFile
)

func NewUserMessage(parts ...ContentBlock) Message {
	return llm.NewUserMessage(parts...)
}

func NewTextMessage(role, text string) Message {
	return llm.NewTextMessage(role, text)
}

func TextBlock(text string) ContentBlock {
	return llm.TextBlock(text)
}

func ImageURLBlockFromURL(url string, detail string) ContentBlock {
	return llm.ImageURLBlockFromURL(url, detail)
}

func ImageBlockFromBase64(data []byte, mimeType string) ContentBlock {
	return llm.ImageBlockFromBase64(data, mimeType)
}

func ImageBlockFromFile(path string) (ContentBlock, error) {
	return llm.ImageBlockFromFile(path)
}

func IsVisionCapableModel(model string) bool {
	return llm.IsVisionCapableModel(model)
}

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
