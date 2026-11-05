package access

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
)

type RunController struct {
	service           *core.Service
	logger            *slog.Logger
	heartbeatInterval time.Duration
}

type RunControllerOptions struct {
	HeartbeatInterval time.Duration
}

func NewRunController(
	service *core.Service,
	logger *slog.Logger,
) *RunController {
	return NewRunControllerWithOptions(
		service,
		logger,
		RunControllerOptions{},
	)
}

func NewRunControllerWithOptions(
	service *core.Service,
	logger *slog.Logger,
	options RunControllerOptions,
) *RunController {
	if options.HeartbeatInterval <= 0 {
		options.HeartbeatInterval = 15 * time.Second
	}
	return &RunController{
		service:           service,
		logger:            logger,
		heartbeatInterval: options.HeartbeatInterval,
	}
}

func (c *RunController) Register(router *httpapi.Router) {
	router.Handle("GET /api/v1/agent/run/stream", c.stream)
	router.Handle("POST /api/v1/agent/run/cancel", c.cancel)
}

func (c *RunController) stream(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.URL.Query().Get("runId"))
	if !validAgentIdentifier(runID) {
		failValidation(w, r, "运行标识无效")
		return
	}
	after := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			failValidation(w, r, "流游标无效")
			return
		}
		after = parsed
	}
	run, err := c.service.GetRun(r.Context(), runID)
	if err != nil {
		writeAgentServiceError(w, r, err, c.logger)
		return
	}
	events, unsubscribe, err := c.service.SubscribeRun(runID, after)
	if err != nil {
		if errors.Is(err, core.ErrJournalCursorExpired) ||
			errors.Is(err, core.ErrJournalClosed) {
			httpapi.Fail(w, r, http.StatusConflict, httpapi.ErrorBody{
				Code:      "AGENT_STREAM_CURSOR_EXPIRED",
				Kind:      "conflict",
				Message:   "实时流游标已过期，请重新加载会话",
				Retryable: true,
			})
			return
		}
		writeAgentServiceError(w, r, err, c.logger)
		return
	}
	defer unsubscribe()

	writeAgentStreamHeaders(w)
	cursor := after
	if cursor < 1 {
		cursor = 1
	}
	if err := writeAgentStreamEvent(w, cursor, uiStartPart{
		MessageID: run.AssistantMessageID,
		MessageMetadata: uiStartMetadata{
			SessionID:          run.SessionID,
			AssistantMessageID: run.AssistantMessageID,
			RunID:              run.ID,
			UserMessageID:      run.UserMessageID,
		},
	}); err != nil {
		c.logger.Error("write agent stream start", "error", err)
		return
	}

	heartbeat := time.NewTicker(c.heartbeatInterval)
	defer heartbeat.Stop()
	terminal := false
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if err := writeAgentStreamHeartbeat(w); err != nil {
				c.logger.Error("write agent stream heartbeat", "error", err)
				return
			}
		case chunk, open := <-events:
			if !open {
				return
			}
			if chunk.Sequence <= cursor {
				continue
			}
			cursor = chunk.Sequence
			part := uiPartFromStreamChunk(chunk)
			if part == nil {
				continue
			}
			if err := writeAgentStreamEvent(w, cursor, part); err != nil {
				c.logger.Error("write agent stream event", "error", err)
				return
			}
			if chunk.Kind == core.StreamFinish || chunk.Kind == core.StreamError {
				terminal = true
			}
			if terminal {
				if err := writeAgentStreamDone(w); err != nil {
					c.logger.Error("write agent stream done", "error", err)
				}
				return
			}
		}
	}
}

type cancelRunRequest struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

func (c *RunController) cancel(w http.ResponseWriter, r *http.Request) {
	var request cancelRunRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil ||
		!validAgentIdentifier(request.SessionID) ||
		!validAgentIdentifier(request.RunID) {
		failValidation(w, r, "停止参数无效")
		return
	}
	run, err := c.service.CancelRun(
		r.Context(),
		request.SessionID,
		request.RunID,
	)
	if err != nil {
		writeAgentServiceError(w, r, err, c.logger)
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]any{
		"runId":  run.ID,
		"status": run.Status,
	})
}

type TestController struct {
	service *core.Service
	logger  *slog.Logger
}

func NewTestController(
	service *core.Service,
	logger *slog.Logger,
) *TestController {
	return &TestController{service: service, logger: logger}
}

func (c *TestController) Register(router *httpapi.Router) {
	router.Handle("POST /test/agent/run/remove-journal", c.removeRunJournal)
}

type removeJournalRequest struct {
	RunID string `json:"runId"`
}

func (c *TestController) removeRunJournal(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request removeJournalRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil ||
		!validAgentIdentifier(request.RunID) {
		failValidation(w, r, "运行标识无效")
		return
	}
	c.service.RemoveRunJournal(request.RunID)
	httpapi.OK(w, http.StatusOK, map[string]bool{"removed": true})
}

func structToAgentError(message string) domain.Error {
	return domain.NewError(
		"AGENT_PROVIDER_FAILED",
		"server",
		message,
		true,
	)
}
