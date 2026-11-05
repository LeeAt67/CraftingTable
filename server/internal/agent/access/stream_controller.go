// Package access 将 Agent 能力适配为 HTTP 和 UI Message Stream。
package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/agent/provider"
	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
)

const (
	maxMessages      = 100
	maxMessageRunes  = 100_000
	maxIdentifier    = 200
	agentStreamRoute = "POST /api/v1/agent/stream"
)

var errUIStreamStopped = errors.New("ui message stream stopped")

// Controller 处理 Agent 聊天流式 HTTP 请求。
type Controller struct {
	provider provider.StreamProvider
	logger   *slog.Logger
}

// NewController 创建 Agent HTTP Controller。
func NewController(
	streamProvider provider.StreamProvider,
	logger *slog.Logger,
) *Controller {
	return &Controller{provider: streamProvider, logger: logger}
}

// Register 注册 Agent 路由。
func (c *Controller) Register(router *httpapi.Router) {
	router.Handle(agentStreamRoute, c.stream)
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamRequest struct {
	SessionID          string        `json:"sessionId"`
	ClientRequestID    string        `json:"clientRequestId"`
	UserMessageID      string        `json:"userMessageId"`
	AssistantMessageID string        `json:"assistantMessageId"`
	AssistantPartID    string        `json:"assistantPartId"`
	RunID              string        `json:"runId"`
	Messages           []chatMessage `json:"messages"`
}

type uiStartMetadata struct {
	SessionID          string `json:"sessionId"`
	AssistantMessageID string `json:"assistantMessageId"`
	RunID              string `json:"runId"`
	UserMessageID      string `json:"userMessageId"`
}

type uiStartPart struct {
	MessageID       string          `json:"messageId"`
	MessageMetadata uiStartMetadata `json:"messageMetadata"`
}

func (p uiStartPart) UIStreamType() string {
	return "start"
}

func (p uiStartPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type            string          `json:"type"`
		MessageID       string          `json:"messageId"`
		MessageMetadata uiStartMetadata `json:"messageMetadata"`
	}{
		Type:            "start",
		MessageID:       p.MessageID,
		MessageMetadata: p.MessageMetadata,
	})
}

type uiFinishMetadata struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}

type uiFinishPart struct {
	FinishReason    string           `json:"finishReason,omitempty"`
	MessageMetadata uiFinishMetadata `json:"messageMetadata"`
}

func (p uiFinishPart) UIStreamType() string {
	return "finish"
}

func (p uiFinishPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type            string           `json:"type"`
		FinishReason    string           `json:"finishReason,omitempty"`
		MessageMetadata uiFinishMetadata `json:"messageMetadata"`
	}{
		Type:            "finish",
		FinishReason:    p.FinishReason,
		MessageMetadata: p.MessageMetadata,
	})
}

type uiErrorPart struct {
	ErrorText  string       `json:"errorText"`
	AgentError domain.Error `json:"agentError"`
}

func (p uiErrorPart) UIStreamType() string {
	return "error"
}

func (p uiErrorPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       string       `json:"type"`
		ErrorText  string       `json:"errorText"`
		AgentError domain.Error `json:"agentError"`
	}{
		Type:       "error",
		ErrorText:  p.ErrorText,
		AgentError: p.AgentError,
	})
}

type uiStartStepPart struct{}

func (uiStartStepPart) UIStreamType() string {
	return "start-step"
}

func (uiStartStepPart) UIStreamJSON() ([]byte, error) {
	return []byte(`{"type":"start-step"}`), nil
}

type uiTextStartPart struct {
	ID string
}

func (uiTextStartPart) UIStreamType() string {
	return "text-start"
}

func (p uiTextStartPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"text-start", p.ID})
}

type uiTextDeltaPart struct {
	ID    string
	Delta string
}

func (uiTextDeltaPart) UIStreamType() string {
	return "text-delta"
}

func (p uiTextDeltaPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Delta string `json:"delta"`
	}{"text-delta", p.ID, p.Delta})
}

type uiTextEndPart struct {
	ID string
}

func (uiTextEndPart) UIStreamType() string {
	return "text-end"
}

func (p uiTextEndPart) UIStreamJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"text-end", p.ID})
}

type uiFinishStepPart struct{}

func (uiFinishStepPart) UIStreamType() string {
	return "finish-step"
}

func (uiFinishStepPart) UIStreamJSON() ([]byte, error) {
	return []byte(`{"type":"finish-step"}`), nil
}

type uiDonePart struct{}

func (uiDonePart) UIStreamType() string {
	return "[DONE]"
}

func (uiDonePart) UIStreamJSON() ([]byte, error) {
	return []byte("[DONE]"), nil
}

type uiMessageStreamPart interface {
	UIStreamType() string
	UIStreamJSON() ([]byte, error)
}

type uiMessageStream func(yield func(uiMessageStreamPart, error) bool)

func (c *Controller) stream(w http.ResponseWriter, r *http.Request) {
	if c.provider == nil {
		httpapi.Fail(w, r, http.StatusServiceUnavailable, httpapi.ErrorBody{
			Code:      "AGENT_NOT_CONFIGURED",
			Kind:      "server",
			Message:   "Agent 模型服务尚未配置",
			Retryable: false,
		})
		return
	}

	request, messages, message := decodeMessages(w, r)
	if message != "" {
		httpapi.Fail(w, r, http.StatusBadRequest, httpapi.ErrorBody{
			Code:      "VALIDATION_ERROR",
			Kind:      "validation",
			Message:   message,
			Retryable: false,
		})
		return
	}

	body, err := c.provider.Stream(r.Context(), messages)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		c.logger.Error(
			"agent provider request failed",
			"request_id", httpapi.RequestID(r),
			"error", err,
		)
		httpapi.Fail(w, r, http.StatusBadGateway, httpapi.ErrorBody{
			Code:      "AGENT_PROVIDER_FAILED",
			Kind:      "server",
			Message:   "模型服务请求失败",
			Retryable: true,
		})
		return
	}
	defer body.Close()

	writeUIMessageStreamHeaders(w)
	if err := relayProviderStream(r.Context(), body, request).pipe(w); err != nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, errUIStreamStopped) {
		c.logger.Error(
			"agent stream relay failed",
			"request_id", httpapi.RequestID(r),
			"error", err,
		)
	}
}

func relayProviderStream(
	ctx context.Context,
	source io.Reader,
	request streamRequest,
) uiMessageStream {
	return func(yield func(uiMessageStreamPart, error) bool) {
		if !yield(uiStartPart{
			MessageID: request.AssistantMessageID,
			MessageMetadata: uiStartMetadata{
				SessionID:          request.SessionID,
				AssistantMessageID: request.AssistantMessageID,
				RunID:              request.RunID,
				UserMessageID:      request.UserMessageID,
			},
		}, nil) {
			return
		}
		if !yield(uiStartStepPart{}, nil) {
			return
		}
		if !yield(uiTextStartPart{ID: request.AssistantPartID}, nil) {
			return
		}

		relayErr := core.RelayProviderStream(
			ctx,
			source,
			func(event string, payload any) error {
				if event != "delta" {
					return nil
				}
				values, ok := payload.(map[string]string)
				if !ok {
					return core.ErrInvalidEvent
				}
				text := values["text"]
				if text == "" {
					return nil
				}
				if !yield(uiTextDeltaPart{
					ID:    request.AssistantPartID,
					Delta: text,
				}, nil) {
					return errUIStreamStopped
				}
				return nil
			},
		)

		if ctx.Err() != nil || errors.Is(relayErr, errUIStreamStopped) {
			return
		}
		if !yield(uiTextEndPart{ID: request.AssistantPartID}, nil) {
			return
		}
		if relayErr != nil {
			agentError := streamError(relayErr)
			if !yield(uiErrorPart{
				ErrorText:  agentError.Message,
				AgentError: agentError,
			}, nil) {
				return
			}
			yield(uiDonePart{}, nil)
			return
		}
		if !yield(uiFinishStepPart{}, nil) {
			return
		}
		if !yield(uiFinishPart{
			FinishReason: "stop",
			MessageMetadata: uiFinishMetadata{
				RunID:  request.RunID,
				Status: "completed",
			},
		}, nil) {
			return
		}
		yield(uiDonePart{}, nil)
	}
}

func writeUIMessageStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")
	w.WriteHeader(http.StatusOK)
}

func (stream uiMessageStream) pipe(w io.Writer) error {
	flusher, _ := w.(http.Flusher)
	var pipeErr error
	stream(func(part uiMessageStreamPart, err error) bool {
		if err != nil {
			pipeErr = err
			return false
		}
		data, err := part.UIStreamJSON()
		if err != nil {
			pipeErr = fmt.Errorf(
				"marshal %s event: %w",
				part.UIStreamType(),
				err,
			)
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			pipeErr = fmt.Errorf("write %s event: %w", part.UIStreamType(), err)
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	})
	return pipeErr
}

func streamError(err error) domain.Error {
	switch {
	case errors.Is(err, core.ErrInvalidEvent):
		return domain.NewError(
			"PROTOCOL_INVALID_METADATA",
			"protocol",
			"模型服务返回了无效事件",
			false,
		)
	case errors.Is(err, core.ErrUnexpectedEOF):
		return domain.NewError(
			"PROTOCOL_UNEXPECTED_EOF",
			"protocol",
			"模型响应意外中断",
			true,
		)
	default:
		return domain.NewError(
			"AGENT_PROVIDER_FAILED",
			"server",
			"模型生成失败",
			true,
		)
	}
}

func decodeMessages(
	w http.ResponseWriter,
	r *http.Request,
) (streamRequest, []provider.Message, string) {
	var request streamRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		return request, nil, "消息参数无效"
	}
	if !validIdentifier(request.SessionID) ||
		!validIdentifier(request.ClientRequestID) ||
		!validIdentifier(request.UserMessageID) ||
		!validIdentifier(request.AssistantMessageID) ||
		!validIdentifier(request.AssistantPartID) ||
		!validIdentifier(request.RunID) {
		return request, nil, "消息关联标识无效"
	}
	if len(request.Messages) == 0 || len(request.Messages) > maxMessages {
		return request, nil, "消息参数无效"
	}

	messages := make([]provider.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			return request, nil, "消息参数无效"
		}
		if strings.TrimSpace(message.Content) == "" ||
			utf8.RuneCountInString(message.Content) > maxMessageRunes {
			return request, nil, "消息参数无效"
		}
		messages = append(messages, provider.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return request, messages, ""
}

func validIdentifier(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= maxIdentifier
}
