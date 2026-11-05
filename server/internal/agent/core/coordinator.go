package core

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var (
	ErrRunAlreadyRegistered = errors.New("agent run already registered")
	ErrRunNotActive         = errors.New("agent run not active")
)

const runJournalLimit = 8 * 1024 * 1024

type RunSnapshot struct {
	Text         string
	LastSequence int
	Status       domain.RunStatus
}

type activeRun struct {
	execution domain.RunExecution
	ctx       context.Context
	cancel    context.CancelFunc
	journal   *EventJournal
}

type Coordinator struct {
	repository Repository
	provider   StreamProvider
	logger     *slog.Logger
	baseCtx    context.Context
	mu         sync.RWMutex
	runs       map[string]*activeRun
}

func NewCoordinator(
	repository Repository,
	provider StreamProvider,
	logger *slog.Logger,
) *Coordinator {
	return &Coordinator{
		repository: repository,
		provider:   provider,
		logger:     logger,
		baseCtx:    context.Background(),
		runs:       make(map[string]*activeRun),
	}
}

func (c *Coordinator) Start(execution domain.RunExecution) error {
	if c.provider == nil {
		return ErrProviderNotConfigured
	}
	c.mu.Lock()
	if _, exists := c.runs[execution.Run.ID]; exists {
		c.mu.Unlock()
		return ErrRunAlreadyRegistered
	}
	ctx, cancel := context.WithCancel(c.baseCtx)
	active := &activeRun{
		execution: execution,
		ctx:       ctx,
		cancel:    cancel,
		journal:   NewEventJournal(runJournalLimit),
	}
	c.runs[execution.Run.ID] = active
	c.mu.Unlock()
	go c.execute(active)
	return nil
}

func (c *Coordinator) Snapshot(runID string) (RunSnapshot, bool) {
	c.mu.RLock()
	active, exists := c.runs[runID]
	c.mu.RUnlock()
	if !exists {
		return RunSnapshot{}, false
	}
	journal := active.journal.Snapshot()
	return RunSnapshot{
		Text:         journal.Text,
		LastSequence: journal.LastSequence,
		Status:       domain.RunRunning,
	}, true
}

func (c *Coordinator) Subscribe(
	runID string,
	after int,
) (<-chan StreamChunk, func(), error) {
	c.mu.RLock()
	active, exists := c.runs[runID]
	c.mu.RUnlock()
	if !exists {
		return nil, func() {}, ErrJournalCursorExpired
	}
	return active.journal.Subscribe(after)
}

func (c *Coordinator) Cancel(runID string) error {
	c.mu.RLock()
	active, exists := c.runs[runID]
	c.mu.RUnlock()
	if !exists {
		return ErrRunNotActive
	}
	active.cancel()
	return nil
}

func (c *Coordinator) Recover(ctx context.Context) error {
	_, err := c.repository.RecoverInterruptedAgentRuns(ctx)
	return err
}

func (c *Coordinator) execute(active *activeRun) {
	execution := active.execution
	for _, chunk := range []StreamChunk{
		{
			Kind:               StreamStart,
			RunID:              execution.Run.ID,
			SessionID:          execution.Run.SessionID,
			UserMessageID:      execution.Run.UserMessageID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
		},
		{
			Kind:               StreamStartStep,
			RunID:              execution.Run.ID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
		},
		{
			Kind:               StreamTextStart,
			RunID:              execution.Run.ID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
		},
	} {
		if err := active.journal.Append(chunk); err != nil {
			c.finishFailure(active, streamFailure(err))
			return
		}
	}

	body, err := c.provider.Stream(
		active.ctx,
		ContextMessages(execution.Messages),
	)
	if err != nil {
		c.finishFailure(active, providerFailure(err))
		return
	}
	defer body.Close()

	err = RelayProviderStream(
		active.ctx,
		body,
		func(event string, payload any) error {
			if event != "delta" {
				return nil
			}
			values, ok := payload.(map[string]string)
			if !ok {
				return ErrInvalidEvent
			}
			return active.journal.Append(StreamChunk{
				Kind:               StreamTextDelta,
				RunID:              execution.Run.ID,
				SessionID:          execution.Run.SessionID,
				UserMessageID:      execution.Run.UserMessageID,
				AssistantMessageID: execution.Run.AssistantMessageID,
				PartID:             execution.AssistantPartID,
				Text:               values["text"],
			})
		},
	)
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			active.ctx.Err() != nil {
			c.finishCancelled(active)
			return
		}
		c.finishFailure(active, streamFailure(err))
		return
	}
	c.finishCompleted(active)
}

func (c *Coordinator) finishCompleted(active *activeRun) {
	execution := active.execution
	text := active.journal.Snapshot().Text
	err := c.repository.FinalizeAgentRun(
		context.Background(),
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunCompleted,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     text,
			MessageStatus:     domain.MessageComplete,
			PartStatus:        domain.PartComplete,
			TerminalEventType: "run.completed",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		return
	}
	c.appendTerminal(active, domain.RunCompleted, nil)
}

func (c *Coordinator) finishCancelled(active *activeRun) {
	execution := active.execution
	text := active.journal.Snapshot().Text
	err := c.repository.FinalizeAgentRun(
		context.Background(),
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunCancelled,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     text,
			MessageStatus:     domain.MessageCancelled,
			PartStatus:        domain.PartCancelled,
			TerminalEventType: "run.cancelled",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		return
	}
	c.appendTerminal(active, domain.RunCancelled, nil)
}

func (c *Coordinator) finishFailure(active *activeRun, agentError domain.Error) {
	execution := active.execution
	text := active.journal.Snapshot().Text
	err := c.repository.FinalizeAgentRun(
		context.Background(),
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunFailed,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     text,
			MessageStatus:     domain.MessageFailed,
			PartStatus:        domain.PartFailed,
			AgentError:        &agentError,
			TerminalEventType: "run.failed",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		return
	}
	c.appendTerminal(active, domain.RunFailed, &agentError)
}

func (c *Coordinator) appendTerminal(
	active *activeRun,
	status domain.RunStatus,
	agentError *domain.Error,
) {
	execution := active.execution
	_ = active.journal.Append(StreamChunk{
		Kind:               StreamTextEnd,
		RunID:              execution.Run.ID,
		SessionID:          execution.Run.SessionID,
		UserMessageID:      execution.Run.UserMessageID,
		AssistantMessageID: execution.Run.AssistantMessageID,
		PartID:             execution.AssistantPartID,
		Status:             status,
	})
	if status == domain.RunFailed {
		_ = active.journal.Append(StreamChunk{
			Kind:               StreamError,
			RunID:              execution.Run.ID,
			SessionID:          execution.Run.SessionID,
			UserMessageID:      execution.Run.UserMessageID,
			AssistantMessageID: execution.Run.AssistantMessageID,
			PartID:             execution.AssistantPartID,
			Status:             status,
			Error:              agentError,
		})
		return
	}
	_ = active.journal.Append(StreamChunk{
		Kind:               StreamFinishStep,
		RunID:              execution.Run.ID,
		AssistantMessageID: execution.Run.AssistantMessageID,
		PartID:             execution.AssistantPartID,
		Status:             status,
	})
	_ = active.journal.Append(StreamChunk{
		Kind:               StreamFinish,
		RunID:              execution.Run.ID,
		SessionID:          execution.Run.SessionID,
		UserMessageID:      execution.Run.UserMessageID,
		AssistantMessageID: execution.Run.AssistantMessageID,
		PartID:             execution.AssistantPartID,
		Status:             status,
	})
}

func (c *Coordinator) logFinalizeError(active *activeRun, err error) {
	if c.logger != nil {
		c.logger.Error(
			"finalize agent run failed",
			"run_id", active.execution.Run.ID,
			"error", err,
		)
	}
}

func providerFailure(err error) domain.Error {
	if errors.Is(err, ErrProviderNotConfigured) {
		return domain.NewError(
			"AGENT_NOT_CONFIGURED",
			"server",
			"Agent 模型服务尚未配置",
			false,
		)
	}
	return domain.NewError(
		"AGENT_PROVIDER_FAILED",
		"server",
		"模型服务请求失败",
		true,
	)
}

func streamFailure(err error) domain.Error {
	switch {
	case errors.Is(err, ErrInvalidEvent):
		return domain.NewError(
			"PROTOCOL_INVALID_METADATA",
			"protocol",
			"模型服务返回了无效事件",
			false,
		)
	case errors.Is(err, ErrUnexpectedEOF):
		return domain.NewError(
			"PROTOCOL_UNEXPECTED_EOF",
			"protocol",
			"模型响应意外中断",
			true,
		)
	case errors.Is(err, ErrJournalFull):
		return domain.NewError(
			"AGENT_STREAM_BUFFER_EXCEEDED",
			"server",
			"模型响应过长，无法继续缓存实时恢复事件",
			false,
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
