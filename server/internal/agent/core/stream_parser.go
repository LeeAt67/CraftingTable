// Package core 实现 Agent 传输协议的纯逻辑处理。
package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

var (
	// ErrUnexpectedEOF 表示模型流在终止事件前结束。
	ErrUnexpectedEOF = errors.New("agent stream ended unexpectedly")
	// ErrInvalidEvent 表示模型流包含非法或不受支持的事件。
	ErrInvalidEvent = errors.New("invalid agent stream event")
	// ErrInvalidToolCall 表示模型返回了不完整的工具调用。
	ErrInvalidToolCall = errors.New("invalid agent tool call")
	// ErrOutputTruncated 表示模型因输出长度限制而中止。
	ErrOutputTruncated = errors.New("agent output truncated")
)

// EventSink 接收规范化后的 Agent SSE 事件。
type EventSink func(event string, payload any) error

type upstreamEvent struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// ProviderTurn is one model step after its streamed fragments are assembled.
type ProviderTurn struct {
	Text         string
	Reasoning    string
	FinishReason string
	ToolCalls    []ProviderToolCall
}

// RelayProviderStream 将 OpenAI 兼容 SSE 转换为 CraftingTable 事件。
func RelayProviderStream(
	ctx context.Context,
	source io.Reader,
	sink EventSink,
) error {
	_, err := relayProviderStream(ctx, source, sink, nil)
	return err
}

// CollectProviderTurn consumes one stream and returns the assembled model turn.
func CollectProviderTurn(
	ctx context.Context,
	source io.Reader,
	sink EventSink,
	onActivity func(),
) (ProviderTurn, error) {
	return relayProviderStream(ctx, source, sink, onActivity)
}

func relayProviderStream(
	ctx context.Context,
	source io.Reader,
	sink EventSink,
	onActivity func(),
) (ProviderTurn, error) {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	dataLines := make([]string, 0, 1)
	turn := ProviderTurn{}
	toolCalls := newToolCallAccumulator()
	for scanner.Scan() {
		if onActivity != nil {
			onActivity()
		}
		if err := ctx.Err(); err != nil {
			return ProviderTurn{}, err
		}

		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			done, err := processData(
				ctx,
				strings.Join(dataLines, "\n"),
				sink,
				&turn,
				toolCalls,
			)
			if err != nil {
				return ProviderTurn{}, err
			}
			if done {
				if err := toolCalls.finalize(&turn); err != nil {
					return ProviderTurn{}, err
				}
				if err := sink("done", map[string]string{
					"status": "completed",
				}); err != nil {
					return ProviderTurn{}, err
				}
				return turn, nil
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" {
				dataLines = append(dataLines, data)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ProviderTurn{}, fmt.Errorf(
			"%w: read provider stream: %v",
			ErrInvalidEvent,
			err,
		)
	}
	if len(dataLines) > 0 {
		done, err := processData(
			ctx,
			strings.Join(dataLines, "\n"),
			sink,
			&turn,
			toolCalls,
		)
		if err != nil {
			return ProviderTurn{}, err
		}
		if done {
			if err := toolCalls.finalize(&turn); err != nil {
				return ProviderTurn{}, err
			}
			if err := sink("done", map[string]string{
				"status": "completed",
			}); err != nil {
				return ProviderTurn{}, err
			}
			return turn, nil
		}
	}
	return ProviderTurn{}, ErrUnexpectedEOF
}

func processData(
	ctx context.Context,
	data string,
	sink EventSink,
	turn *ProviderTurn,
	toolCalls *toolCallAccumulator,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if data == "[DONE]" {
		if toolCalls.len() > 0 {
			turn.FinishReason = "tool_calls"
		} else {
			turn.FinishReason = "stop"
		}
		return true, nil
	}

	var event upstreamEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return false, fmt.Errorf("%w: decode JSON: %v", ErrInvalidEvent, err)
	}
	if len(event.Choices) == 0 {
		return false, nil
	}

	choice := event.Choices[0]
	if choice.Delta.ReasoningContent != "" {
		if err := sink("reasoning", map[string]string{
			"text": choice.Delta.ReasoningContent,
		}); err != nil {
			return false, err
		}
		turn.Reasoning += choice.Delta.ReasoningContent
	}
	if choice.Delta.Content != "" {
		if err := sink("delta", map[string]string{
			"text": choice.Delta.Content,
		}); err != nil {
			return false, err
		}
		turn.Text += choice.Delta.Content
	}
	for _, fragment := range choice.Delta.ToolCalls {
		if err := toolCalls.add(fragment); err != nil {
			return false, err
		}
	}
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		switch *choice.FinishReason {
		case "stop":
			if toolCalls.len() > 0 {
				return false, fmt.Errorf(
					"%w: stop finish reason included tool calls",
					ErrInvalidToolCall,
				)
			}
			turn.FinishReason = "stop"
		case "tool_calls":
			if toolCalls.len() == 0 {
				return false, fmt.Errorf(
					"%w: tool_calls finish reason had no calls",
					ErrInvalidToolCall,
				)
			}
			turn.FinishReason = "tool_calls"
		case "length":
			return false, ErrOutputTruncated
		default:
			return false, fmt.Errorf(
				"%w: unsupported finish reason %q",
				ErrInvalidEvent,
				*choice.FinishReason,
			)
		}
		return true, nil
	}
	return false, nil
}

type toolCallFragment = struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type accumulatedToolCall struct {
	index     int
	id        string
	typeName  string
	name      string
	arguments strings.Builder
	hasID     bool
	hasType   bool
	hasName   bool
}

type toolCallAccumulator struct {
	calls map[int]*accumulatedToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[int]*accumulatedToolCall)}
}

func (a *toolCallAccumulator) len() int {
	return len(a.calls)
}

func (a *toolCallAccumulator) add(fragment toolCallFragment) error {
	call, exists := a.calls[fragment.Index]
	if !exists {
		call = &accumulatedToolCall{index: fragment.Index}
		a.calls[fragment.Index] = call
	}
	if fragment.ID != "" {
		if call.hasID && call.id != fragment.ID {
			return fmt.Errorf("%w: conflicting tool call id", ErrInvalidToolCall)
		}
		call.id = fragment.ID
		call.hasID = true
	}
	if fragment.Type != "" {
		if call.hasType && call.typeName != fragment.Type {
			return fmt.Errorf("%w: conflicting tool call type", ErrInvalidToolCall)
		}
		call.typeName = fragment.Type
		call.hasType = true
	}
	if fragment.Function.Name != "" {
		if call.hasName && call.name != fragment.Function.Name {
			return fmt.Errorf("%w: conflicting tool call name", ErrInvalidToolCall)
		}
		call.name = fragment.Function.Name
		call.hasName = true
	}
	call.arguments.WriteString(fragment.Function.Arguments)
	return nil
}

func (a *toolCallAccumulator) finalize(turn *ProviderTurn) error {
	if len(a.calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(a.calls))
	for index := range a.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	turn.ToolCalls = make([]ProviderToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := a.calls[index]
		if !call.hasID || !call.hasName {
			return fmt.Errorf("%w: tool call is missing id or name", ErrInvalidToolCall)
		}
		arguments := call.arguments.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		var document any
		if err := json.Unmarshal([]byte(arguments), &document); err != nil {
			return fmt.Errorf("%w: invalid arguments JSON", ErrInvalidToolCall)
		}
		if _, ok := document.(map[string]any); !ok {
			return fmt.Errorf("%w: arguments must be an object", ErrInvalidToolCall)
		}
		typeName := call.typeName
		if typeName == "" {
			typeName = "function"
		}
		turn.ToolCalls = append(turn.ToolCalls, ProviderToolCall{
			ID:   call.id,
			Type: typeName,
			Function: ProviderFunctionCall{
				Name:      call.name,
				Arguments: json.RawMessage(arguments),
			},
		})
	}
	return nil
}
