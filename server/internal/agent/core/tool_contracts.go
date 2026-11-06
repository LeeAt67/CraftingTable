package core

import (
	"context"
	"encoding/json"
	"time"
)

// ToolDefinition describes one immutable Agent tool.
type ToolDefinition struct {
	Name           string
	Namespace      string
	Version        string
	Title          string
	Description    string
	InputSchema    json.RawMessage
	ReadOnly       bool
	Idempotent     bool
	Timeout        time.Duration
	MaxInputBytes  int
	MaxResultBytes int
}

// ToolHandler executes one validated tool call.
type ToolHandler func(context.Context, json.RawMessage) (ToolResult, error)

// Tool binds a definition to its handler.
type Tool struct {
	Definition ToolDefinition
	Handler    ToolHandler
}

// ToolCall is the provider-neutral representation of one requested tool call.
type ToolCall struct {
	ID             string
	ProviderCallID string
	RunID          string
	StepID         string
	Name           string
	Version        string
	Arguments      json.RawMessage
	ArgumentsHash  string
}

// ToolResultContent is one content block returned by a tool.
type ToolResultContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	JSON any    `json:"json,omitempty"`
}

// ToolResult is the provider-neutral result of one tool call.
type ToolResult struct {
	CallID       string              `json:"callId,omitempty"`
	Content      []ToolResultContent `json:"content"`
	IsError      bool                `json:"isError"`
	Error        *ToolError          `json:"error,omitempty"`
	Truncated    bool                `json:"truncated"`
	EncodedBytes int                 `json:"encodedBytes"`
	CreatedAt    string              `json:"createdAt,omitempty"`
}

// ToolError is a stable error safe to return to the model.
type ToolError struct {
	Code      string `json:"code"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Hint      string `json:"hint,omitempty"`
}

// Error implements error.
func (e ToolError) Error() string {
	return e.Message
}

// ProviderToolDefinition is an OpenAI-compatible function tool definition.
type ProviderToolDefinition struct {
	Type     string               `json:"type"`
	Function ProviderFunctionTool `json:"function"`
}

// ProviderFunctionTool is the function payload of a provider tool definition.
type ProviderFunctionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
