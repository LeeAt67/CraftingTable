package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var (
	ErrRunAlreadyRegistered = errors.New("agent run already registered")
	ErrRunNotActive         = errors.New("agent run not active")
	ErrToolLimitExceeded    = errors.New("agent tool limit exceeded")
	ErrModelStepLimit       = errors.New("agent model step limit exceeded")
	ErrToolExecutionFailed  = errors.New("agent tool execution failed")
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
	registry   *ToolRegistry
	tools      *ToolExecutor
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
	return newCoordinator(repository, provider, nil, logger, config)
}

func newCoordinator(
	repository Repository,
	provider StreamProvider,
	registry *ToolRegistry,
	logger *slog.Logger,
	config RuntimeConfig,
) *Coordinator {
	var tools *ToolExecutor
	if registry != nil {
		tools = NewToolExecutor(registry)
	}
	return &Coordinator{
		repository: repository,
		provider:   provider,
		registry:   registry,
		tools:      tools,
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
	if s.execution.ReasoningPartID == "" || text == "" || s.reasoningEnded {
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
	if err := c.executeModelToolLoop(active); err != nil {
		c.finishFromExecutionError(active, err)
		return
	}
	c.finishSuccess(active)
}

func (c *Coordinator) executeModelToolLoop(active *activeRun) error {
	execution := active.execution
	messages := ContextMessages(execution.Messages)
	modelStepID := execution.ModelStepID
	modelSteps := 0
	toolCallsUsed := 0
	seenProviderCallIDs := make(map[string]struct{})

	for {
		if modelSteps >= c.config.MaxModelSteps {
			return ErrModelStepLimit
		}

		request := ModelRequest{Messages: messages}
		if c.config.ToolsEnabled && c.tools != nil && c.registry != nil {
			request.Tools = c.registry.ProviderTools()
			request.ToolChoice = "auto"
		}
		body, err := c.provider.Stream(active.ctx, request)
		if err != nil {
			return err
		}
		turn, collectErr := CollectProviderTurn(
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
		closeErr := body.Close()
		if collectErr != nil {
			return collectErr
		}
		if closeErr != nil {
			return fmt.Errorf("%w: close provider stream: %v", ErrUnexpectedEOF, closeErr)
		}
		modelSteps++

		if len(turn.ToolCalls) == 0 {
			if err := c.completeModelStep(
				active,
				modelStepID,
				"completed",
				"",
			); err != nil {
				return toolExecutionFailure(err)
			}
			return nil
		}
		if !c.config.ToolsEnabled || c.tools == nil {
			return ErrInvalidToolCall
		}
		if toolCallsUsed+len(turn.ToolCalls) > c.config.MaxToolCalls {
			return ErrToolLimitExceeded
		}
		for _, call := range turn.ToolCalls {
			if call.Type != "" && call.Type != "function" {
				return fmt.Errorf(
					"%w: unsupported tool call type %q",
					ErrInvalidToolCall,
					call.Type,
				)
			}
			if _, exists := seenProviderCallIDs[call.ID]; exists {
				return fmt.Errorf(
					"%w: duplicate provider call id %q",
					ErrInvalidToolCall,
					call.ID,
				)
			}
			seenProviderCallIDs[call.ID] = struct{}{}
		}
		if err := c.completeModelStep(
			active,
			modelStepID,
			"completed",
			"",
		); err != nil {
			return toolExecutionFailure(err)
		}

		messages = append(messages, ProviderMessage{
			Role:      "assistant",
			Content:   turn.Text,
			ToolCalls: turn.ToolCalls,
		})
		for _, providerCall := range turn.ToolCalls {
			message, err := c.executeToolCall(
				active,
				modelStepID,
				providerCall,
			)
			if err != nil {
				return err
			}
			messages = append(messages, ProviderMessage{
				Role:       "tool",
				Content:    message,
				ToolCallID: providerCall.ID,
			})
			toolCallsUsed++
		}

		if modelSteps >= c.config.MaxModelSteps {
			return ErrModelStepLimit
		}
		nextStep, err := c.repository.CreateAgentModelStep(
			active.ctx,
			CreateAgentModelStepRecord{RunID: execution.Run.ID},
		)
		if err != nil {
			return toolExecutionFailure(err)
		}
		modelStepID = nextStep.ID
	}
}

func (c *Coordinator) executeToolCall(
	active *activeRun,
	parentStepID string,
	providerCall ProviderToolCall,
) (string, error) {
	version := c.toolVersion(providerCall.Function.Name)
	arguments := providerCall.Function.Arguments
	argumentsHash := hashToolArguments(arguments)
	if c.registry != nil {
		if canonical, toolErr := c.registry.Validate(
			providerCall.Function.Name,
			version,
			arguments,
		); toolErr == nil {
			arguments = canonical
			argumentsHash = hashToolArguments(canonical)
		}
	}
	step, err := c.repository.CreateAgentToolCall(
		active.ctx,
		BeginAgentToolCallRecord{
			RunID:          active.execution.Run.ID,
			ParentStepID:   parentStepID,
			ProviderCallID: providerCall.ID,
			ToolName:       providerCall.Function.Name,
			ToolVersion:    version,
			Arguments:      arguments,
			ArgumentsHash:  argumentsHash,
		},
	)
	if err != nil {
		return "", toolExecutionFailure(err)
	}

	callID := serverToolCallID(
		active.execution.Run.ID,
		providerCall.ID,
	)
	result, err := c.tools.Execute(active.ctx, ToolCall{
		ID:             callID,
		ProviderCallID: providerCall.ID,
		RunID:          active.execution.Run.ID,
		StepID:         step.ID,
		Name:           providerCall.Function.Name,
		Version:        version,
		Arguments:      arguments,
		ArgumentsHash:  argumentsHash,
	})
	if err != nil {
		return "", err
	}
	if err := c.repository.CompleteAgentToolCall(
		active.ctx,
		CompleteAgentToolCallRecord{
			RunID:        active.execution.Run.ID,
			StepID:       step.ID,
			Result:       result,
			EncodedBytes: result.EncodedBytes,
		},
	); err != nil {
		return "", toolExecutionFailure(err)
	}
	content, err := encodeToolResultContext(result)
	if err != nil {
		return "", toolExecutionFailure(err)
	}
	return content, nil
}

func (c *Coordinator) completeModelStep(
	active *activeRun,
	stepID string,
	status string,
	errorCode string,
) error {
	return c.repository.CompleteAgentModelStep(
		active.ctx,
		CompleteAgentModelStepRecord{
			RunID:     active.execution.Run.ID,
			StepID:    stepID,
			Status:    status,
			ErrorCode: errorCode,
		},
	)
}

func (c *Coordinator) toolVersion(name string) string {
	if c.registry != nil {
		if registered, exists := c.registry.tools[name]; exists {
			return registered.tool.Definition.Version
		}
	}
	return "1.0.0"
}

func toolExecutionFailure(err error) error {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrToolExecutionFailed, err)
}

func hashToolArguments(arguments json.RawMessage) string {
	hash := sha256.Sum256(arguments)
	return hex.EncodeToString(hash[:])
}

func serverToolCallID(runID string, providerCallID string) string {
	hash := sha256.Sum256([]byte(runID + "\x00" + providerCallID))
	return hex.EncodeToString(hash[:])
}

func encodeToolResultContext(result ToolResult) (string, error) {
	content := result.Content
	if content == nil {
		content = make([]ToolResultContent, 0)
	}
	body, err := json.Marshal(struct {
		Content   []ToolResultContent `json:"content"`
		IsError   bool                `json:"isError"`
		Error     *ToolError          `json:"error,omitempty"`
		Truncated bool                `json:"truncated"`
	}{
		Content:   content,
		IsError:   result.IsError,
		Error:     result.Error,
		Truncated: result.Truncated,
	})
	if err != nil {
		return "", fmt.Errorf("encode tool result context: %w", err)
	}
	return string(body), nil
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
	case errors.Is(err, ErrOutputTruncated):
		return domain.NewError(
			"AGENT_OUTPUT_TRUNCATED",
			"protocol",
			"模型输出达到长度限制",
			true,
		)
	case errors.Is(err, ErrInvalidToolCall):
		return domain.NewError(
			"AGENT_TOOL_CALL_INVALID",
			"protocol",
			"模型返回了无效的工具调用",
			false,
		)
	case errors.Is(err, ErrToolLimitExceeded):
		return domain.NewError(
			"AGENT_TOOL_LIMIT_EXCEEDED",
			"server",
			"工具调用次数超过限制",
			true,
		)
	case errors.Is(err, ErrModelStepLimit):
		return domain.NewError(
			"AGENT_MODEL_STEP_LIMIT_EXCEEDED",
			"server",
			"模型续轮次数超过限制",
			true,
		)
	case errors.Is(err, ErrToolExecutionFailed):
		return domain.NewError(
			"AGENT_TOOL_EXECUTION_FAILED",
			"server",
			"工具执行失败",
			true,
		)
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
