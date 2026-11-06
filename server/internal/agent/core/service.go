package core

import (
	"context"
	"log/slog"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

type Service struct {
	repository  Repository
	coordinator *Coordinator
	logger      *slog.Logger
	config      RuntimeConfig
}

func NewService(
	repository Repository,
	provider StreamProvider,
	logger *slog.Logger,
) *Service {
	return NewServiceWithConfig(
		repository,
		provider,
		logger,
		DefaultRuntimeConfig(),
	)
}

func NewServiceWithConfig(
	repository Repository,
	provider StreamProvider,
	logger *slog.Logger,
	config RuntimeConfig,
) *Service {
	return &Service{
		repository:  repository,
		coordinator: NewCoordinator(repository, provider, logger, config),
		logger:      logger,
		config:      normalizedRuntimeConfig(config),
	}
}

func NewServiceWithTools(
	repository Repository,
	provider StreamProvider,
	registry *ToolRegistry,
	logger *slog.Logger,
	config RuntimeConfig,
) *Service {
	return &Service{
		repository: repository,
		coordinator: newCoordinator(
			repository,
			provider,
			registry,
			logger,
			config,
		),
		logger: logger,
		config: normalizedRuntimeConfig(config),
	}
}

func (s *Service) CreateSession(
	ctx context.Context,
	input CreateSessionRecord,
) (domain.Session, error) {
	return s.repository.CreateAgentSession(ctx, input)
}

func (s *Service) ListSessions(
	ctx context.Context,
) ([]domain.Session, error) {
	return s.repository.ListAgentSessions(ctx)
}

func (s *Service) GetSession(
	ctx context.Context,
	sessionID string,
) (domain.SessionDetail, error) {
	detail, err := s.repository.GetAgentSession(ctx, sessionID)
	if err != nil {
		return domain.SessionDetail{}, err
	}
	if detail.ActiveRun == nil || detail.ActiveRun.Status != domain.RunRunning {
		return detail, nil
	}
	snapshot, found := s.coordinator.Snapshot(detail.ActiveRun.RunID)
	if !found {
		return detail, nil
	}
	detail.ActiveRun.LastSequence = snapshot.LastSequence
	for index := range detail.MessageParts {
		part := &detail.MessageParts[index]
		if part.MessageID != detail.ActiveRun.AssistantMessageID ||
			part.RunID == nil ||
			*part.RunID != detail.ActiveRun.RunID {
			continue
		}
		switch part.Type {
		case domain.PartText:
			part.Text = snapshot.Text
		case domain.PartReasoning:
			part.Text = snapshot.Reasoning
		}
	}
	return detail, nil
}

func (s *Service) SendMessage(
	ctx context.Context,
	input CreateSendRunRecord,
) (domain.RunHandle, error) {
	execution, err := s.repository.CreateAgentSendRun(ctx, input)
	if err != nil {
		return domain.RunHandle{}, err
	}
	if execution.Replayed {
		return domain.RunHandle{}, replayedRunError(execution)
	}
	if err := s.coordinator.Start(execution); err != nil {
		s.failStartup(execution)
		return domain.RunHandle{}, err
	}
	return handleFromExecution(execution), nil
}

func (s *Service) RetryMessage(
	ctx context.Context,
	input CreateRetryRunRecord,
) (domain.RunHandle, error) {
	execution, err := s.repository.CreateAgentRetryRun(ctx, input)
	if err != nil {
		return domain.RunHandle{}, err
	}
	if execution.Replayed {
		return domain.RunHandle{}, replayedRunError(execution)
	}
	if err := s.coordinator.Start(execution); err != nil {
		s.failStartup(execution)
		return domain.RunHandle{}, err
	}
	return handleFromExecution(execution), nil
}

func (s *Service) CancelRun(
	ctx context.Context,
	sessionID string,
	runID string,
) (domain.Run, error) {
	run, err := s.repository.GetAgentRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if run.SessionID != sessionID {
		return domain.Run{}, ErrRunNotFound
	}
	if run.Status != domain.RunRunning {
		return run, nil
	}
	status, err := s.coordinator.Cancel(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	run.Status = status
	return run, nil
}

func (s *Service) SubscribeRun(
	runID string,
	after int,
) (<-chan StreamChunk, func(), error) {
	return s.coordinator.Subscribe(runID, after)
}

func (s *Service) GetRun(
	ctx context.Context,
	runID string,
) (domain.Run, error) {
	return s.repository.GetAgentRun(ctx, runID)
}

func (s *Service) Recover(ctx context.Context) error {
	return s.coordinator.Recover(ctx)
}

func (s *Service) RemoveRunJournal(runID string) {
	s.coordinator.RemoveJournal(runID)
}

func handleFromExecution(execution domain.RunExecution) domain.RunHandle {
	return domain.RunHandle{
		RunID:              execution.Run.ID,
		SessionID:          execution.Run.SessionID,
		UserMessageID:      execution.Run.UserMessageID,
		AssistantMessageID: execution.Run.AssistantMessageID,
		AssistantPartID:    execution.AssistantPartID,
		ReasoningPartID:    execution.ReasoningPartID,
		Status:             execution.Run.Status,
	}
}

func replayedRunError(execution domain.RunExecution) error {
	code := ErrRequestAlreadyProcessed
	if execution.Run.Status == domain.RunRunning {
		code = ErrSessionBusy
	}
	return &RunStateError{
		Err:    code,
		RunID:  execution.Run.ID,
		Status: execution.Run.Status,
	}
}

func (s *Service) failStartup(execution domain.RunExecution) {
	agentError := domain.Error{
		Code:      "AGENT_RUN_START_FAILED",
		Kind:      "server",
		Message:   "Agent 执行启动失败",
		Retryable: true,
	}
	err := finalizeRunWithRetry(
		s.repository,
		s.config,
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunFailed,
			AssistantPartID:   execution.AssistantPartID,
			MessageStatus:     domain.MessageFailed,
			PartStatus:        domain.PartFailed,
			AgentError:        &agentError,
			TerminalEventType: "run.failed",
		},
	)
	if err == nil {
		return
	}
	if s.logger != nil {
		s.logger.Error(
			"finalize agent run after startup failure",
			"run_id", execution.Run.ID,
			"error", err,
		)
	}
	_ = interruptRunWithRetry(
		s.repository,
		s.config,
		InterruptRunRecord{
			RunID:           execution.Run.ID,
			AssistantPartID: execution.AssistantPartID,
			AgentError:      agentError,
		},
	)
}
