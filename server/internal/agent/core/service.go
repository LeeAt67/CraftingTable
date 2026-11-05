package core

import (
	"context"
	"log/slog"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

type Service struct {
	repository  Repository
	coordinator *Coordinator
}

func NewService(
	repository Repository,
	provider StreamProvider,
	logger *slog.Logger,
) *Service {
	return &Service{
		repository:  repository,
		coordinator: NewCoordinator(repository, provider, logger),
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
		if part.MessageID == detail.ActiveRun.AssistantMessageID &&
			part.RunID != nil &&
			*part.RunID == detail.ActiveRun.RunID &&
			part.Type == domain.PartText {
			part.Text = snapshot.Text
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
	if err := s.coordinator.Start(execution); err != nil {
		_ = s.repository.FinalizeAgentRun(ctx, FinalizeRunRecord{
			RunID:           execution.Run.ID,
			Status:          domain.RunFailed,
			AssistantPartID: execution.AssistantPartID,
			MessageStatus:   domain.MessageFailed,
			PartStatus:      domain.PartFailed,
			AgentError: &domain.Error{
				Code:      "AGENT_RUN_START_FAILED",
				Kind:      "server",
				Message:   "Agent 执行启动失败",
				Retryable: true,
			},
			TerminalEventType: "run.failed",
		})
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
	if err := s.coordinator.Start(execution); err != nil {
		_ = s.repository.FinalizeAgentRun(ctx, FinalizeRunRecord{
			RunID:           execution.Run.ID,
			Status:          domain.RunFailed,
			AssistantPartID: execution.AssistantPartID,
			MessageStatus:   domain.MessageFailed,
			PartStatus:      domain.PartFailed,
			AgentError: &domain.Error{
				Code:      "AGENT_RUN_START_FAILED",
				Kind:      "server",
				Message:   "Agent 执行启动失败",
				Retryable: true,
			},
			TerminalEventType: "run.failed",
		})
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
	if err := s.coordinator.Cancel(runID); err != nil {
		return domain.Run{}, err
	}
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
		Status:             execution.Run.Status,
	}
}
