package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var ErrProviderNotConfigured = errors.New("agent provider not configured")

type ProviderMessage struct {
	Role       string             `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCalls  []ProviderToolCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

// ProviderToolCall is one assistant-requested function call.
type ProviderToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function ProviderFunctionCall `json:"function"`
}

// ProviderFunctionCall carries a function name and JSON arguments.
type ProviderFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// MarshalJSON encodes function arguments as the string required on the wire.
func (c ProviderFunctionCall) MarshalJSON() ([]byte, error) {
	arguments := string(c.Arguments)
	return json.Marshal(struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments,omitempty"`
	}{
		Name:      c.Name,
		Arguments: arguments,
	})
}

// ModelRequest is one provider-neutral model call.
type ModelRequest struct {
	Messages   []ProviderMessage        `json:"messages"`
	Tools      []ProviderToolDefinition `json:"tools,omitempty"`
	ToolChoice string                   `json:"tool_choice,omitempty"`
}

type StreamProvider interface {
	Stream(context.Context, ModelRequest) (io.ReadCloser, error)
}

func ContextMessages(messages []domain.ContextMessage) []ProviderMessage {
	result := make([]ProviderMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, ProviderMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return result
}
