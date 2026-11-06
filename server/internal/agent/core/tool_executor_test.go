package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolExecutorValidatesBeforeInvokingHandler(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	tool := testTool("validated")
	tool.Handler = func(context.Context, json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		return ToolResult{}, nil
	}
	registry, err := NewToolRegistry([]Tool{tool})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	executor := NewToolExecutor(registry)

	result, err := executor.Execute(context.Background(), ToolCall{
		Name:      "validated",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{"name":42}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("handler calls = %d, want 0", calls.Load())
	}
	if !result.IsError || result.Error == nil || result.Error.Kind != "validation" {
		t.Fatalf("Execute() result = %#v, want validation error", result)
	}
}

func TestToolExecutorReturnsCanonicalArgumentsToHandler(t *testing.T) {
	t.Parallel()

	var received string
	tool := testTool("canonical")
	tool.Handler = func(_ context.Context, arguments json.RawMessage) (ToolResult, error) {
		received = string(arguments)
		return ToolResult{
			Content: []ToolResultContent{{Type: "text", Text: "ok"}},
		}, nil
	}
	registry, err := NewToolRegistry([]Tool{tool})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	result, err := NewToolExecutor(registry).Execute(context.Background(), ToolCall{
		Name:      "canonical",
		Version:   "1.0.0",
		Arguments: json.RawMessage("{ \"name\" : \"ok\" }"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() result = %#v", result)
	}
	if received != `{"name":"ok"}` {
		t.Fatalf("handler arguments = %q, want canonical JSON", received)
	}
}

func TestToolExecutorReturnsNotFoundForUnknownTool(t *testing.T) {
	t.Parallel()

	registry, err := NewToolRegistry(nil)
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	result, err := NewToolExecutor(registry).Execute(context.Background(), ToolCall{
		Name:      "missing",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.IsError || result.Error == nil || result.Error.Kind != "not_found" {
		t.Fatalf("Execute() result = %#v, want not_found", result)
	}
}

func TestToolExecutorRecoversHandlerPanicAsInternalResult(t *testing.T) {
	t.Parallel()

	tool := testTool("panics")
	tool.Handler = func(context.Context, json.RawMessage) (ToolResult, error) {
		panic("hidden implementation detail")
	}
	registry, err := NewToolRegistry([]Tool{tool})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	result, err := NewToolExecutor(registry).Execute(context.Background(), ToolCall{
		Name:      "panics",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{"name":"ok"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.IsError || result.Error == nil || result.Error.Kind != "internal" {
		t.Fatalf("Execute() result = %#v, want internal error", result)
	}
	if strings.Contains(result.Error.Message, "hidden implementation detail") {
		t.Fatalf("panic detail leaked to ToolResult: %#v", result.Error)
	}
}

func TestToolExecutorPropagatesRunCancellation(t *testing.T) {
	t.Parallel()

	registry, err := NewToolRegistry([]Tool{testTool("cancelled")})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = NewToolExecutor(registry).Execute(ctx, ToolCall{
		Name:      "cancelled",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{"name":"ok"}`),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestToolExecutorReturnsTimeoutForToolDeadline(t *testing.T) {
	t.Parallel()

	tool := testTool("slow")
	tool.Definition.Timeout = 10 * time.Millisecond
	tool.Handler = func(ctx context.Context, _ json.RawMessage) (ToolResult, error) {
		<-ctx.Done()
		return ToolResult{}, ctx.Err()
	}
	registry, err := NewToolRegistry([]Tool{tool})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	result, err := NewToolExecutor(registry).Execute(context.Background(), ToolCall{
		Name:      "slow",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{"name":"ok"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want ToolResult timeout", err)
	}
	if !result.IsError || result.Error == nil || result.Error.Kind != "timeout" {
		t.Fatalf("Execute() result = %#v, want timeout", result)
	}
}

func TestToolExecutorTruncatesOversizedResult(t *testing.T) {
	t.Parallel()

	tool := testTool("oversized")
	tool.Definition.MaxResultBytes = 128
	tool.Handler = func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{
			Content: []ToolResultContent{{
				Type: "text",
				Text: strings.Repeat("x", 1024),
			}},
		}, nil
	}
	registry, err := NewToolRegistry([]Tool{tool})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	result, err := NewToolExecutor(registry).Execute(context.Background(), ToolCall{
		Name:      "oversized",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{"name":"ok"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Truncated {
		t.Fatalf("Execute() result was not marked truncated: %#v", result)
	}
	if result.EncodedBytes > tool.Definition.MaxResultBytes {
		t.Fatalf(
			"encoded bytes = %d, max = %d",
			result.EncodedBytes,
			tool.Definition.MaxResultBytes,
		)
	}
}

func TestToolExecutorRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	tool := testTool("small-input")
	tool.Definition.MaxInputBytes = 8
	tool.Handler = func(context.Context, json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		return ToolResult{}, nil
	}
	registry, err := NewToolRegistry([]Tool{tool})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	result, err := NewToolExecutor(registry).Execute(context.Background(), ToolCall{
		Name:      "small-input",
		Version:   "1.0.0",
		Arguments: json.RawMessage(`{"name":"too large"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("handler calls = %d, want 0", calls.Load())
	}
	if !result.IsError || result.Error == nil || result.Error.Kind != "validation" {
		t.Fatalf("Execute() result = %#v, want validation", result)
	}
}
