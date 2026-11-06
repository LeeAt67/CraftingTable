package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ToolExecutor validates and executes registered tools.
type ToolExecutor struct {
	registry *ToolRegistry
}

// NewToolExecutor creates an executor over an immutable registry.
func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return &ToolExecutor{registry: registry}
}

// Execute runs one tool serially and returns a stable ToolResult.
func (e *ToolExecutor) Execute(
	ctx context.Context,
	call ToolCall,
) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	if e == nil || e.registry == nil {
		return toolErrorResult(call, notFoundToolError(call.Name)), nil
	}
	registered, exists := e.registry.Resolve(call.Name, call.Version)
	if !exists {
		return toolErrorResult(call, notFoundToolError(call.Name)), nil
	}
	definition := registered.Definition
	if len(call.Arguments) > definition.MaxInputBytes {
		return toolErrorResult(
			call,
			validationToolError("工具参数超过大小限制"),
		), nil
	}
	canonical, toolError := e.registry.Validate(
		call.Name,
		call.Version,
		call.Arguments,
	)
	if toolError != nil {
		return toolErrorResult(call, toolError), nil
	}
	hash := sha256.Sum256(canonical)
	call.Arguments = canonical
	call.ArgumentsHash = hex.EncodeToString(hash[:])

	toolCtx, cancel := context.WithTimeout(ctx, definition.Timeout)
	defer cancel()
	type handlerOutcome struct {
		result ToolResult
		err    error
	}
	finished := make(chan handlerOutcome, 1)
	go func() {
		result, handlerErr := invokeToolHandler(
			toolCtx,
			registered.Handler,
			canonical,
		)
		finished <- handlerOutcome{result: result, err: handlerErr}
	}()

	var (
		result     ToolResult
		handlerErr error
	)
	select {
	case outcome := <-finished:
		result = outcome.result
		handlerErr = outcome.err
	case <-toolCtx.Done():
		if err := ctx.Err(); err != nil {
			return ToolResult{}, err
		}
		return encodeToolResult(
			toolErrorResult(call, timeoutToolError()),
			call,
			definition.MaxResultBytes,
		)
	}
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	if errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
		result = toolErrorResult(call, timeoutToolError())
	} else if handlerErr != nil {
		var stable ToolError
		if errors.As(handlerErr, &stable) {
			result = toolErrorResult(call, &stable)
		} else {
			result = toolErrorResult(call, internalToolError())
		}
	}
	if validateToolResultContent(result.Content) != nil {
		result = toolErrorResult(call, internalToolError())
	}
	return encodeToolResult(result, call, definition.MaxResultBytes)
}

func invokeToolHandler(
	ctx context.Context,
	handler ToolHandler,
	arguments json.RawMessage,
) (result ToolResult, err error) {
	defer func() {
		if recover() != nil {
			err = internalToolError()
		}
	}()
	return handler(ctx, arguments)
}

func validateToolResultContent(content []ToolResultContent) error {
	for _, block := range content {
		switch block.Type {
		case "text", "json":
		default:
			return fmt.Errorf("unsupported tool result content type %q", block.Type)
		}
	}
	return nil
}

func toolErrorResult(call ToolCall, toolError *ToolError) ToolResult {
	return ToolResult{
		CallID:  call.ID,
		Content: nil,
		IsError: true,
		Error:   toolError,
	}
}

func encodeToolResult(
	result ToolResult,
	call ToolCall,
	maxBytes int,
) (ToolResult, error) {
	result.CallID = call.ID
	if result.CreatedAt == "" {
		result.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(result)
	if err != nil {
		result = toolErrorResult(call, internalToolError())
		result.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		body, err = json.Marshal(result)
		if err != nil {
			return ToolResult{}, fmt.Errorf("encode internal tool error: %w", err)
		}
	}
	if len(body) > maxBytes {
		result.Content = []ToolResultContent{{
			Type: "text",
			Text: "Tool result truncated.",
		}}
		result.Truncated = true
		result.EncodedBytes = 0
		body, err = json.Marshal(result)
		if err != nil {
			return ToolResult{}, fmt.Errorf("encode truncated tool result: %w", err)
		}
		for len(body) > maxBytes && result.Content[0].Text != "" {
			runes := []rune(result.Content[0].Text)
			result.Content[0].Text = string(runes[:len(runes)-1])
			body, err = json.Marshal(result)
			if err != nil {
				return ToolResult{}, fmt.Errorf(
					"encode truncated tool result: %w",
					err,
				)
			}
		}
	}
	result.EncodedBytes = len(body)
	return result, nil
}
