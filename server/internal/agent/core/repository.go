package core

import (
	"context"
	"errors"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var (
	ErrSessionNotFound         = errors.New("agent session not found")
	ErrRunNotFound             = errors.New("agent run not found")
	ErrSessionBusy             = errors.New("agent session busy")
	ErrIdempotencyConflict     = errors.New("agent idempotency conflict")
	ErrMessageNotRetryable     = errors.New("agent message not retryable")
	ErrRequestAlreadyProcessed = errors.New("agent request already processed")
)

type RunStateError struct {
	Err    error
	RunID  string
	Status domain.RunStatus
}

func (e *RunStateError) Error() string {
	return e.Err.Error()
}

func (e *RunStateError) Unwrap() error {
	return e.Err
}

type RequestIDConflictError struct {
	ClientRequestID string
}

func (e *RequestIDConflictError) Error() string {
	return "agent request id conflict"
}

func (e *RequestIDConflictError) Unwrap() error {
	return ErrIdempotencyConflict
}

type CreateSessionRecord struct {
	ClientRequestID string
	RequestHash     string
}

type CreateSendRunRecord struct {
	SessionID       string
	ClientRequestID string
	RequestHash     string
	UserMessageID   string
	UserParts       []string
}

type CreateRetryRunRecord struct {
	SessionID          string
	AssistantMessageID string
	ClientRequestID    string
	RequestHash        string
}

type FinalizeRunRecord struct {
	RunID              string
	Status             domain.RunStatus
	AssistantPartID    string
	AssistantText      string
	MessageStatus      domain.MessageStatus
	PartStatus         domain.PartStatus
	AgentError         *domain.Error
	TerminalEventType  string
	CompletedEventType string
}

type InterruptRunRecord struct {
	RunID           string
	AssistantPartID string
	AssistantText   string
	AgentError      domain.Error
}

type Repository interface {
	CreateAgentSession(context.Context, CreateSessionRecord) (domain.Session, error)
	ListAgentSessions(context.Context) ([]domain.Session, error)
	GetAgentSession(context.Context, string) (domain.SessionDetail, error)
	CreateAgentSendRun(context.Context, CreateSendRunRecord) (domain.RunExecution, error)
	CreateAgentRetryRun(context.Context, CreateRetryRunRecord) (domain.RunExecution, error)
	FinalizeAgentRun(context.Context, FinalizeRunRecord) error
	InterruptAgentRun(context.Context, InterruptRunRecord) error
	GetAgentRun(context.Context, string) (domain.Run, error)
	RecoverInterruptedAgentRuns(context.Context) (int, error)
}
