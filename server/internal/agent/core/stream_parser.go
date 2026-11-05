// Package core 实现 Agent 传输协议的纯逻辑处理。
package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	// ErrUnexpectedEOF 表示模型流在终止事件前结束。
	ErrUnexpectedEOF = errors.New("agent stream ended unexpectedly")
	// ErrInvalidEvent 表示模型流包含非法或不受支持的事件。
	ErrInvalidEvent = errors.New("invalid agent stream event")
)

// EventSink 接收规范化后的 Agent SSE 事件。
type EventSink func(event string, payload any) error

type upstreamEvent struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// RelayProviderStream 将 OpenAI 兼容 SSE 转换为 CraftingTable 事件。
func RelayProviderStream(
	ctx context.Context,
	source io.Reader,
	sink EventSink,
) error {
	return relayProviderStream(ctx, source, sink, nil)
}

func relayProviderStream(
	ctx context.Context,
	source io.Reader,
	sink EventSink,
	onActivity func(),
) error {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	dataLines := make([]string, 0, 1)
	for scanner.Scan() {
		if onActivity != nil {
			onActivity()
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			done, err := processData(ctx, strings.Join(dataLines, "\n"), sink)
			if err != nil {
				return err
			}
			if done {
				return nil
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
		return fmt.Errorf("%w: read provider stream: %v", ErrInvalidEvent, err)
	}
	if len(dataLines) > 0 {
		done, err := processData(ctx, strings.Join(dataLines, "\n"), sink)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return ErrUnexpectedEOF
}

func processData(
	ctx context.Context,
	data string,
	sink EventSink,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if data == "[DONE]" {
		if err := sink("done", map[string]string{"status": "completed"}); err != nil {
			return false, err
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
	if choice.Delta.Content != "" {
		if err := sink("delta", map[string]string{
			"text": choice.Delta.Content,
		}); err != nil {
			return false, err
		}
	}
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		if err := sink("done", map[string]string{"status": "completed"}); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
