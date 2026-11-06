package core

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var (
	ErrRunAlreadyRegistered = errors.New("agent run already registered")
	ErrRunNotActive         = errors.New("agent run not active")
)

const runJournalLimit = 8 * 1024 * 1024

type RunSnapshot struct {
	Text         string
	Reasoning    string
	LastSequence int
	Status       domain.RunStatus
}

type activeRun struct {
	execution       domain.RunExecution
	ctx             context.Context
	cancel          context.CancelFunc
	journal         *EventJournal
	stream          *runStream
	terminalMu      sync.Mutex
	cancelRequested bool
	finishClaimed   bool
	finishedStatus  domain.RunStatus
	done            chan struct{}
	idleTimedOut    atomic.Bool
	idleMu          sync.Mutex
	idleTimer       *time.Timer
	idleTimeout     time.Duration
}

type Coordinator struct {
	repository Repository
	provider   StreamProvider
	logger     *slog.Logger
	baseCtx    context.Context
	mu         sync.RWMutex
	runs       map[string]*activeRun
	config     RuntimeConfig
}

func NewCoordinator(
	repository Repository,
	provider StreamProvider,
	logger *slog.Logger,
	config RuntimeConfig,
) *Coordinator {
	return &Coordinator{
		repository: repository,
		provider:   provider,
		logger:     logger,
		baseCtx:    context.Background(),
		runs:       make(map[string]*activeRun),
		config:     normalizedRuntimeConfig(config),
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
	ctx, cancel := context.WithTimeout(
		c.baseCtx,
		c.config.MaxRunDuration,
	)
	active := &activeRun{
		execution:   execution,
		ctx:         ctx,
		cancel:      cancel,
		journal:     NewEventJournal(runJournalLimit),
		idleTimeout: c.config.IdleTimeout,
		done:        make(chan struct{}),
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
		Reasoning:    journal.Reasoning,
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

func (c *Coordinator) Cancel(
	ctx context.Context,
	runID string,
) (domain.RunStatus, error) {
	c.mu.RLock()
	active, exists := c.runs[runID]
	c.mu.RUnlock()
	if !exists {
		return "", ErrRunNotActive
	}
	active.terminalMu.Lock()
	if active.finishClaimed {
		done := active.done
		active.terminalMu.Unlock()
		select {
		case <-done:
			active.terminalMu.Lock()
			status := active.finishedStatus
			active.terminalMu.Unlock()
			return status, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	active.cancelRequested = true
	active.terminalMu.Unlock()
	active.cancel()
	return domain.RunRunning, nil
}

func (c *Coordinator) RemoveJournal(runID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if active, exists := c.runs[runID]; exists {
		active.journal.Close()
		delete(c.runs, runID)
	}
}

func (c *Coordinator) Recover(ctx context.Context) error {
	_, err := c.repository.RecoverInterruptedAgentRuns(ctx)
	return err
}

type runStream struct {
	active           *activeRun
	execution        domain.RunExecution
	reasoningStarted bool
	reasoningEnded   bool
	textStarted      bool
	textEnded        bool
}

func newRunStream(
	active *activeRun,
	execution domain.RunExecution,
) *runStream {
	return &runStream{active: active, execution: execution}
}

func (s *runStream) chunk(kind StreamChunkKind, partID string) StreamChunk {
	return StreamChunk{
		Kind:               kind,
		RunID:              s.execution.Run.ID,
		SessionID:          s.execution.Run.SessionID,
		UserMessageID:      s.execution.Run.UserMessageID,
		AssistantMessageID: s.execution.Run.AssistantMessageID,
		PartID:             partID,
	}
}

func (s *runStream) reasoning(text string) error {
	if s.execution.ReasoningPartID == "" || text == "" {
		return nil
	}
	if !s.reasoningStarted {
		if err := s.active.journal.Append(s.chunk(
			StreamReasoningStart,
			s.execution.ReasoningPartID,
		)); err != nil {
			return err
		}
		s.reasoningStarted = true
	}
	delta := s.chunk(StreamReasoningDelta, s.execution.ReasoningPartID)
	delta.Text = text
	return s.active.journal.Append(delta)
}

func (s *runStream) endReasoning() error {
	if !s.reasoningStarted || s.reasoningEnded {
		return nil
	}
	s.reasoningEnded = true
	return s.active.journal.Append(s.chunk(
		StreamReasoningEnd,
		s.execution.ReasoningPartID,
	))
}

func (s *runStream) text(text string) error {
	if text == "" {
		return nil
	}
	if err := s.endReasoning(); err != nil {
		return err
	}
	if !s.textStarted {
		if err := s.active.journal.Append(s.chunk(
			StreamTextStart,
			s.execution.AssistantPartID,
		)); err != nil {
			return err
		}
		s.textStarted = true
	}
	delta := s.chunk(StreamTextDelta, s.execution.AssistantPartID)
	delta.Text = text
	return s.active.journal.Append(delta)
}

func (s *runStream) close() error {
	if err := s.endReasoning(); err != nil {
		return err
	}
	if !s.textStarted || s.textEnded {
		return nil
	}
	s.textEnded = true
	return s.active.journal.Append(s.chunk(
		StreamTextEnd,
		s.execution.AssistantPartID,
	))
}

func (c *Coordinator) execute(active *activeRun) {
	defer close(active.done)
	defer c.stopRun(active)
	execution := active.execution
	active.stream = newRunStream(active, execution)
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
	} {
		if err := active.journal.Append(chunk); err != nil {
			c.finishFailure(active, streamFailure(err))
			return
		}
	}

	active.startIdleTimer(c.config.IdleTimeout)
	body, err := c.provider.Stream(
		active.ctx,
		ModelRequest{Messages: ContextMessages(execution.Messages)},
	)
	if err != nil {
		c.finishFromExecutionError(active, err)
		return
	}
	defer body.Close()

	_, err = CollectProviderTurn(
		active.ctx,
		body,
		func(event string, payload any) error {
			values, ok := payload.(map[string]string)
			if !ok {
				return ErrInvalidEvent
			}
			switch event {
			case "reasoning":
				return active.stream.reasoning(values["text"])
			case "delta":
				return active.stream.text(values["text"])
			default:
				return nil
			}
		},
		active.touch,
	)
	if err != nil {
		c.finishFromExecutionError(active, err)
		return
	}
	c.finishSuccess(active)
}

func (c *Coordinator) finishSuccess(active *activeRun) {
	status, claimed := active.claimFinish(domain.RunCompleted)
	if !claimed {
		return
	}
	switch status {
	case domain.RunCancelled:
		c.finishCancelled(active)
	case domain.RunFailed:
		c.finishFailure(active, providerTimeoutFailure())
	default:
		c.finishCompleted(active)
	}
}

func (c *Coordinator) finishCompleted(active *activeRun) {
	execution := active.execution
	snapshot := active.journal.Snapshot()
	err := c.finalize(
		active,
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunCompleted,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     snapshot.Text,
			ReasoningPartID:   execution.ReasoningPartID,
			ReasoningText:     snapshot.Reasoning,
			MessageStatus:     domain.MessageComplete,
			PartStatus:        domain.PartComplete,
			TerminalEventType: "run.completed",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		c.finishInterrupted(active)
		return
	}
	c.appendTerminal(active, domain.RunCompleted, nil)
	c.scheduleJournalRemoval(active)
}

func (c *Coordinator) finishCancelled(active *activeRun) {
	execution := active.execution
	snapshot := active.journal.Snapshot()
	err := c.finalize(
		active,
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunCancelled,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     snapshot.Text,
			ReasoningPartID:   execution.ReasoningPartID,
			ReasoningText:     snapshot.Reasoning,
			MessageStatus:     domain.MessageCancelled,
			PartStatus:        domain.PartCancelled,
			TerminalEventType: "run.cancelled",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		c.finishInterrupted(active)
		return
	}
	c.appendTerminal(active, domain.RunCancelled, nil)
	c.scheduleJournalRemoval(active)
}

func (c *Coordinator) finishFailure(active *activeRun, agentError domain.Error) {
	execution := active.execution
	snapshot := active.journal.Snapshot()
	err := c.finalize(
		active,
		FinalizeRunRecord{
			RunID:             execution.Run.ID,
			Status:            domain.RunFailed,
			AssistantPartID:   execution.AssistantPartID,
			AssistantText:     snapshot.Text,
			ReasoningPartID:   execution.ReasoningPartID,
			ReasoningText:     snapshot.Reasoning,
			MessageStatus:     domain.MessageFailed,
			PartStatus:        domain.PartFailed,
			AgentError:        &agentError,
			TerminalEventType: "run.failed",
		},
	)
	if err != nil {
		c.logFinalizeError(active, err)
		c.finishInterrupted(active)
		return
	}
	c.appendTerminal(active, domain.RunFailed, &agentError)
	c.scheduleJournalRemoval(active)
}

func (c *Coordinator) finishInterrupted(active *activeRun) {
	active.fallbackInterrupted()
	execution := active.execution
	snapshot := active.journal.Snapshot()
	agentError := domain.NewError(
		"AGENT_FINALIZE_FAILED",
		"server",
		"Agent 运行终结失败",
		true,
	)
	err := interruptRunWithRetry(
		c.repository,
		c.config,
		InterruptRunRecord{
			RunID:           execution.Run.ID,
			AssistantPartID: execution.AssistantPartID,
			AssistantText:   snapshot.Text,
			ReasoningPartID: execution.ReasoningPartID,
			ReasoningText:   snapshot.Reasoning,
			AgentError:      agentError,
		},
	)
	if err != nil {
		if c.logger != nil {
			c.logger.Error(
				"interrupt agent run after finalize failure",
				"run_id", execution.Run.ID,
				"error", err,
			)
		}
		c.scheduleJournalRemoval(active)
		return
	}
	c.appendTerminal(active, domain.RunInterrupted, &agentError)
	c.scheduleJournalRemoval(active)
}

func (c *Coordinator) appendTerminal(
	active *activeRun,
	status domain.RunStatus,
	agentError *domain.Error,
) {
	execution := active.execution
	if active.stream != nil {
		_ = active.stream.close()
	}
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

func (c *Coordinator) finalize(
	active *activeRun,
	record FinalizeRunRecord,
) error {
	return finalizeRunWithRetry(c.repository, c.config, record)
}

func (c *Coordinator) finishFromExecutionError(
	active *activeRun,
	err error,
) {
	preferredStatus := domain.RunFailed
	agentError := streamFailure(err)
	switch {
	case active.idleTimedOut.Load() ||
		errors.Is(active.ctx.Err(), context.DeadlineExceeded):
		agentError = providerTimeoutFailure()
	case errors.Is(err, context.Canceled):
		preferredStatus = domain.RunCancelled
		agentError = domain.Error{}
	}
	status, claimed := active.claimFinish(preferredStatus)
	if !claimed {
		return
	}
	if status == domain.RunCancelled {
		c.finishCancelled(active)
		return
	}
	c.finishFailure(active, agentError)
}

func (c *Coordinator) scheduleJournalRemoval(active *activeRun) {
	runID := active.execution.Run.ID
	time.AfterFunc(c.config.TerminalJournalTTL, func() {
		c.RemoveJournal(runID)
	})
}

func (c *Coordinator) stopRun(active *activeRun) {
	active.idleMu.Lock()
	if active.idleTimer != nil {
		active.idleTimer.Stop()
	}
	active.idleMu.Unlock()
	active.cancel()
}

func (a *activeRun) claimFinish(
	preferred domain.RunStatus,
) (domain.RunStatus, bool) {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	if a.finishClaimed {
		return a.finishedStatus, false
	}
	status := preferred
	switch {
	case a.cancelRequested:
		status = domain.RunCancelled
	case a.idleTimedOut.Load() ||
		errors.Is(a.ctx.Err(), context.DeadlineExceeded):
		status = domain.RunFailed
	}
	a.finishClaimed = true
	a.finishedStatus = status
	return status, true
}

func (a *activeRun) fallbackInterrupted() {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	a.finishedStatus = domain.RunInterrupted
}

func (a *activeRun) startIdleTimer(timeout time.Duration) {
	a.idleMu.Lock()
	defer a.idleMu.Unlock()
	if a.idleTimer != nil {
		a.idleTimer.Stop()
	}
	a.idleTimeout = timeout
	a.idleTimer = time.AfterFunc(timeout, func() {
		a.idleTimedOut.Store(true)
		a.cancel()
	})
}

func (a *activeRun) touch() {
	a.idleMu.Lock()
	defer a.idleMu.Unlock()
	if a.idleTimer != nil {
		a.idleTimer.Reset(a.idleTimeout)
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

func providerTimeoutFailure() domain.Error {
	return domain.NewError(
		"AGENT_PROVIDER_TIMEOUT",
		"server",
		"模型服务响应超时",
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
