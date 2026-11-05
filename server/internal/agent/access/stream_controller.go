// Package access 将 Agent 能力适配为 HTTP 和 SSE。
package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	agentStreamRoute = "POST /api/v1/agent/stream"
)

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
	Messages []chatMessage `json:"messages"`
}

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

	messages, message := decodeMessages(w, r)
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpapi.Fail(w, r, http.StatusInternalServerError, httpapi.ErrorBody{
			Code:      "INTERNAL_ERROR",
			Kind:      "unknown",
			Message:   "当前连接不支持流式响应",
			Retryable: true,
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	err = core.RelayProviderStream(
		r.Context(),
		body,
		func(event string, payload any) error {
			return writeEvent(w, flusher, event, payload)
		},
	)
	if err == nil || errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}

	switch {
	case errors.Is(err, core.ErrInvalidEvent):
		_ = writeEvent(w, flusher, "error", domain.NewError(
			"PROTOCOL_INVALID_METADATA",
			"protocol",
			"模型服务返回了无效事件",
			false,
		))
	case errors.Is(err, core.ErrUnexpectedEOF):
		_ = writeEvent(w, flusher, "error", domain.NewError(
			"PROTOCOL_UNEXPECTED_EOF",
			"protocol",
			"模型响应意外中断",
			true,
		))
	default:
		c.logger.Error(
			"agent stream relay failed",
			"request_id", httpapi.RequestID(r),
			"error", err,
		)
		_ = writeEvent(w, flusher, "error", domain.NewError(
			"AGENT_PROVIDER_FAILED",
			"server",
			"模型生成失败",
			true,
		))
	}
}

func decodeMessages(
	w http.ResponseWriter,
	r *http.Request,
) ([]provider.Message, string) {
	var request streamRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		return nil, "消息参数无效"
	}
	if len(request.Messages) == 0 || len(request.Messages) > maxMessages {
		return nil, "消息参数无效"
	}

	messages := make([]provider.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			return nil, "消息参数无效"
		}
		if strings.TrimSpace(message.Content) == "" ||
			utf8.RuneCountInString(message.Content) > maxMessageRunes {
			return nil, "消息参数无效"
		}
		messages = append(messages, provider.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return messages, ""
}

func writeEvent(
	w http.ResponseWriter,
	flusher http.Flusher,
	event string,
	payload any,
) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", event, err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return fmt.Errorf("write %s event: %w", event, err)
	}
	flusher.Flush()
	return nil
}
