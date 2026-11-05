package core

import (
	"context"
	"errors"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var (
	ErrSessionNotFound     = errors.New("agent session not found")
	ErrRunNotFound         = errors.New("agent run not found")
	ErrSessionBusy         = errors.New("agent session busy")
	ErrIdempotencyConflict = errors.New("agent idempotency conflict")
	ErrMessageNotRetryable = errors.New("agent message not retryable")
)

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

type Repository interface {
	CreateAgentSession(context.Context, CreateSessionRecord) (domain.Session, error)
	ListAgentSessions(context.Context) ([]domain.Session, error)
	GetAgentSession(context.Context, string) (domain.SessionDetail, error)
	CreateAgentSendRun(context.Context, CreateSendRunRecord) (domain.RunExecution, error)
	CreateAgentRetryRun(context.Context, CreateRetryRunRecord) (domain.RunExecution, error)
	FinalizeAgentRun(context.Context, FinalizeRunRecord) error
	GetAgentRun(context.Context, string) (domain.Run, error)
	RecoverInterruptedAgentRuns(context.Context) (int, error)
}
