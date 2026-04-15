package tools

import "anyclaw/pkg/sdk"

// Tool is the internal alias for the public tool contract.
type Tool = sdk.Tool

// ToolRegistry is the internal alias for the public tool registry contract.
type ToolRegistry = sdk.ToolRegistry

// ToolSpec is the tool declaration shape.
type ToolSpec = sdk.ToolSpec

// ToolCall is a structured tool invocation request.
type ToolCall = sdk.ToolCall

// ToolResult is the normalized tool invocation result.
type ToolResult = sdk.ToolResult
