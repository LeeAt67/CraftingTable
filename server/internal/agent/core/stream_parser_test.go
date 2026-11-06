package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type capturedEvent struct {
	name    string
	payload any
}

func TestRelayProviderStreamNormalizesDeltasAndDone(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"，Leeat\"}}]}\n\n" +
			"data: [DONE]\n\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].name != "delta" || events[1].name != "delta" {
		t.Fatalf("first events = %#v, want two delta events", events[:2])
	}
	if events[2].name != "done" {
		t.Fatalf("last event = %q, want done", events[2].name)
	}
}

func TestRelayProviderStreamSeparatesReasoningFromContent(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先分析\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"最终回答\"}}]}\n\n" +
			"data: [DONE]\n\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].name != "reasoning" {
		t.Fatalf("first event = %q, want reasoning", events[0].name)
	}
	if values, ok := events[0].payload.(map[string]string); !ok ||
		values["text"] != "先分析" {
		t.Fatalf("reasoning payload = %#v", events[0].payload)
	}
	if events[1].name != "delta" {
		t.Fatalf("second event = %q, want delta", events[1].name)
	}
}

func TestRelayProviderStreamAcceptsFinalEventWithoutBlankLine(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
			"data: [DONE]\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 2 || events[1].name != "done" {
		t.Fatalf("events = %#v, want delta then done", events)
	}
}

func TestRelayProviderStreamHonorsFinishReason(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 2 || events[1].name != "done" {
		t.Fatalf("events = %#v, want delta then done", events)
	}
}

func TestRelayProviderStreamRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	source := strings.NewReader("data: not-json\n\n")
	err := RelayProviderStream(
		context.Background(),
		source,
		func(string, any) error { return nil },
	)
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error = %v, want ErrInvalidEvent", err)
	}
}

func TestRelayProviderStreamRejectsUnexpectedEOF(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"尚未完成\"}}]}\n\n",
	)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(string, any) error { return nil },
	)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want ErrUnexpectedEOF", err)
	}
}

func TestCollectProviderTurnAccumulatesFragmentedToolCalls(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"today_search\",\"arguments\":\"{\\\"date\\\":\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"2026-11-06\\\"}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
	)
	turn, err := CollectProviderTurn(
		context.Background(),
		source,
		func(string, any) error { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("CollectProviderTurn() error = %v", err)
	}
	if turn.FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q, want tool_calls", turn.FinishReason)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v, want 1", turn.ToolCalls)
	}
	call := turn.ToolCalls[0]
	if call.ID != "call-1" ||
		call.Type != "function" ||
		call.Function.Name != "today_search" ||
		string(call.Function.Arguments) != `{"date":"2026-11-06"}` {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestCollectProviderTurnPreservesMultipleToolCallOrder(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call-2\",\"type\":\"function\",\"function\":{\"name\":\"today_summary\",\"arguments\":\"{}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"today_search\",\"arguments\":\"{}\"}}]}}]}\n\n" +
			"data: [DONE]\n\n",
	)
	turn, err := CollectProviderTurn(
		context.Background(),
		source,
		func(string, any) error { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("CollectProviderTurn() error = %v", err)
	}
	if turn.FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q, want inferred tool_calls", turn.FinishReason)
	}
	if len(turn.ToolCalls) != 2 ||
		turn.ToolCalls[0].ID != "call-1" ||
		turn.ToolCalls[1].ID != "call-2" {
		t.Fatalf("tool call order = %#v", turn.ToolCalls)
	}
}

func TestCollectProviderTurnRejectsInvalidToolArguments(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"today_search\",\"arguments\":\"not-json\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
	)
	_, err := CollectProviderTurn(
		context.Background(),
		source,
		func(string, any) error { return nil },
		nil,
	)
	if !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("error = %v, want ErrInvalidToolCall", err)
	}
}

func TestCollectProviderTurnRejectsConflictingToolFragments(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"today_search\",\"arguments\":\"{}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-2\",\"function\":{\"arguments\":\"{}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
	)
	_, err := CollectProviderTurn(
		context.Background(),
		source,
		func(string, any) error { return nil },
		nil,
	)
	if !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("error = %v, want ErrInvalidToolCall", err)
	}
}

func TestCollectProviderTurnMapsFinishReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantReason string
		wantErr    error
	}{
		{
			name:       "stop",
			source:     "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n",
			wantReason: "stop",
		},
		{
			name:    "length",
			source:  "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n",
			wantErr: ErrOutputTruncated,
		},
		{
			name:    "unknown",
			source:  "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\n",
			wantErr: ErrInvalidEvent,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			turn, err := CollectProviderTurn(
				context.Background(),
				strings.NewReader(test.source),
				func(string, any) error { return nil },
				nil,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && turn.FinishReason != test.wantReason {
				t.Fatalf(
					"finish reason = %q, want %q",
					turn.FinishReason,
					test.wantReason,
				)
			}
		})
	}
}
