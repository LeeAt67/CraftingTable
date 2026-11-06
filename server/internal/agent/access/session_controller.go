package access

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
)

const (
	agentMaxIdentifier = 200
	agentMaxPartRunes  = 100_000
)

type SessionController struct {
	service *core.Service
	logger  *slog.Logger
}

func NewSessionController(
	service *core.Service,
	logger *slog.Logger,
) *SessionController {
	return &SessionController{service: service, logger: logger}
}

func (c *SessionController) Register(router *httpapi.Router) {
	router.Handle("POST /api/v1/agent/session/create", c.createSession)
	router.Handle("GET /api/v1/agent/session/list", c.listSessions)
	router.Handle("GET /api/v1/agent/session/get", c.getSession)
	router.Handle("POST /api/v1/agent/message/send", c.sendMessage)
	router.Handle("POST /api/v1/agent/message/retry", c.retryMessage)
}

type createSessionRequest struct {
	ClientRequestID string `json:"clientRequestId"`
}

func (c *SessionController) createSession(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request createSessionRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil ||
		!validAgentIdentifier(request.ClientRequestID) {
		failValidation(w, r, "会话参数无效")
		return
	}
	hash, err := requestHash(request)
	if err != nil {
		c.fail(w, r, err)
		return
	}
	session, err := c.service.CreateSession(r.Context(), core.CreateSessionRecord{
		ClientRequestID: request.ClientRequestID,
		RequestHash:     hash,
	})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]any{"session": session})
}

func (c *SessionController) listSessions(
	w http.ResponseWriter,
	r *http.Request,
) {
	sessions, err := c.service.ListSessions(r.Context())
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, sessions)
}

func (c *SessionController) getSession(
	w http.ResponseWriter,
	r *http.Request,
) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	if !validAgentIdentifier(sessionID) {
		failValidation(w, r, "会话标识无效")
		return
	}
	detail, err := c.service.GetSession(r.Context(), sessionID)
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, sessionDetailResponse{
		Session:           detail.Session,
		Messages:          detail.Messages,
		MessageParts:      messagePartResponses(detail.MessageParts),
		ActiveRun:         detail.ActiveRun,
		NextMessageCursor: detail.NextMessageCursor,
	})
}

type sendMessageRequest struct {
	SessionID       string              `json:"sessionId"`
	ClientRequestID string              `json:"clientRequestId"`
	Message         sendMessageEnvelope `json:"message"`
}

type sendMessageEnvelope struct {
	ID    string            `json:"id"`
	Role  string            `json:"role"`
	Parts []sendMessagePart `json:"parts"`
}

type sendMessagePart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (c *SessionController) sendMessage(w http.ResponseWriter, r *http.Request) {
	var request sendMessageRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		failValidation(w, r, "消息参数无效")
		return
	}
	parts, message := validateSendMessage(request)
	if message != "" {
		failValidation(w, r, message)
		return
	}
	hash, err := requestHash(struct {
		SessionID       string              `json:"sessionId"`
		ClientRequestID string              `json:"clientRequestId"`
		Message         sendMessageEnvelope `json:"message"`
	}{
		SessionID:       request.SessionID,
		ClientRequestID: request.ClientRequestID,
		Message:         request.Message,
	})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	handle, err := c.service.SendMessage(r.Context(), core.CreateSendRunRecord{
		SessionID:       request.SessionID,
		ClientRequestID: request.ClientRequestID,
		RequestHash:     hash,
		UserMessageID:   request.Message.ID,
		UserParts:       parts,
	})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, handle)
}

type retryMessageRequest struct {
	SessionID          string `json:"sessionId"`
	AssistantMessageID string `json:"assistantMessageId"`
	ClientRequestID    string `json:"clientRequestId"`
}

func (c *SessionController) retryMessage(w http.ResponseWriter, r *http.Request) {
	var request retryMessageRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil ||
		!validAgentIdentifier(request.SessionID) ||
		!validAgentIdentifier(request.AssistantMessageID) ||
		!validAgentIdentifier(request.ClientRequestID) {
		failValidation(w, r, "重试参数无效")
		return
	}
	hash, err := requestHash(request)
	if err != nil {
		c.fail(w, r, err)
		return
	}
	handle, err := c.service.RetryMessage(r.Context(), core.CreateRetryRunRecord{
		SessionID:          request.SessionID,
		AssistantMessageID: request.AssistantMessageID,
		ClientRequestID:    request.ClientRequestID,
		RequestHash:        hash,
	})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, handle)
}

func validateSendMessage(request sendMessageRequest) ([]string, string) {
	if !validAgentIdentifier(request.SessionID) ||
		!validAgentIdentifier(request.ClientRequestID) ||
		!validAgentIdentifier(request.Message.ID) {
		return nil, "消息关联标识无效"
	}
	if request.Message.Role != string(domain.MessageUser) ||
		len(request.Message.Parts) == 0 ||
		len(request.Message.Parts) > 20 {
		return nil, "消息参数无效"
	}
	parts := make([]string, 0, len(request.Message.Parts))
	totalRunes := 0
	for _, part := range request.Message.Parts {
		partRunes := utf8.RuneCountInString(part.Text)
		if part.Type != string(domain.PartText) ||
			strings.TrimSpace(part.Text) == "" ||
			partRunes > agentMaxPartRunes {
			return nil, "消息正文无效"
		}
		totalRunes += partRunes
		if totalRunes > agentMaxPartRunes {
			return nil, "消息正文总长度超出限制"
		}
		parts = append(parts, part.Text)
	}
	return parts, ""
}

func validAgentIdentifier(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= agentMaxIdentifier
}

func requestHash(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

type sessionDetailResponse struct {
	Session           domain.Session        `json:"session"`
	Messages          []domain.Message      `json:"messages"`
	MessageParts      []messagePartResponse `json:"messageParts"`
	ActiveRun         *domain.ActiveRun     `json:"activeRun"`
	NextMessageCursor *string               `json:"nextMessageCursor"`
}

type messagePartResponse struct {
	ID        string             `json:"id"`
	MessageID string             `json:"messageId"`
	RunID     *string            `json:"runId,omitempty"`
	Index     int                `json:"index"`
	Type      domain.PartType    `json:"type"`
	Status    domain.PartStatus  `json:"status"`
	Content   messagePartContent `json:"content"`
	CreatedAt string             `json:"createdAt"`
	UpdatedAt string             `json:"updatedAt"`
}

type messagePartContent struct {
	Type  domain.PartType `json:"type"`
	Text  string          `json:"text"`
	Error *domain.Error   `json:"error,omitempty"`
}

func messagePartResponses(
	parts []domain.MessagePart,
) []messagePartResponse {
	responses := make([]messagePartResponse, 0, len(parts))
	for _, part := range parts {
		responses = append(responses, messagePartResponse{
			ID:        part.ID,
			MessageID: part.MessageID,
			RunID:     part.RunID,
			Index:     part.Index,
			Type:      part.Type,
			Status:    part.Status,
			Content: messagePartContent{
				Type:  part.Type,
				Text:  part.Text,
				Error: part.Error,
			},
			CreatedAt: part.CreatedAt,
			UpdatedAt: part.UpdatedAt,
		})
	}
	return responses
}

func failValidation(w http.ResponseWriter, r *http.Request, message string) {
	httpapi.Fail(w, r, http.StatusBadRequest, httpapi.ErrorBody{
		Code:      "VALIDATION_ERROR",
		Kind:      "validation",
		Message:   message,
		Retryable: false,
	})
}

func (c *SessionController) fail(w http.ResponseWriter, r *http.Request, err error) {
	writeAgentServiceError(w, r, err, c.logger)
}

func writeAgentServiceError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	logger *slog.Logger,
) {
	status := http.StatusInternalServerError
	body := httpapi.ErrorBody{
		Code:      "AGENT_PERSISTENCE_FAILED",
		Kind:      "server",
		Message:   "Agent 数据保存失败",
		Retryable: true,
	}
	var runState *core.RunStateError
	if errors.As(err, &runState) {
		body.Details = map[string]any{
			"runId":  runState.RunID,
			"status": runState.Status,
		}
	}
	var requestConflict *core.RequestIDConflictError
	if errors.As(err, &requestConflict) {
		body.Details = map[string]any{
			"clientRequestId": requestConflict.ClientRequestID,
		}
	}
	switch {
	case errors.Is(err, core.ErrSessionNotFound):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusNotFound, "AGENT_SESSION_NOT_FOUND", "not_found",
			"会话不存在或已被删除", false
	case errors.Is(err, core.ErrRunNotFound):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusNotFound, "AGENT_RUN_NOT_FOUND", "not_found",
			"Agent 运行不存在", false
	case errors.Is(err, core.ErrSessionBusy):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusConflict, "AGENT_SESSION_BUSY", "conflict",
			"当前会话仍有回复正在生成", false
	case errors.Is(err, core.ErrRequestAlreadyProcessed):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusConflict, "AGENT_REQUEST_ALREADY_PROCESSED", "conflict",
			"该请求已经处理完成", false
	case errors.Is(err, core.ErrIdempotencyConflict):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusConflict, "AGENT_REQUEST_ID_CONFLICT", "conflict",
			"相同请求标识对应了不同请求内容", false
	case errors.Is(err, core.ErrMessageNotRetryable):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusConflict, "AGENT_MESSAGE_NOT_RETRYABLE", "conflict",
			"当前回复不能重试", false
	case errors.Is(err, core.ErrProviderNotConfigured):
		status, body.Code, body.Kind, body.Message, body.Retryable =
			http.StatusServiceUnavailable, "AGENT_NOT_CONFIGURED", "server",
			"Agent 模型服务尚未配置", false
	}
	if logger != nil && status >= http.StatusInternalServerError {
		logger.Error(
			"agent persistence request failed",
			"request_id", httpapi.RequestID(r),
			"error", err,
		)
	}
	httpapi.Fail(w, r, status, body)
}
