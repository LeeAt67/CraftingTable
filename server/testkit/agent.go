// Package testkit exposes the internal Agent persistence boundary to the
// repository's integration tests without making implementation packages public.
package testkit

import (
	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/agent/provider"
	"github.com/leeat/craftingtable/server/internal/agent/tools"
	"github.com/leeat/craftingtable/server/internal/platform/store"
	"github.com/leeat/craftingtable/server/internal/today"
)

type SQLiteStore = store.SQLiteStore

type CreateAgentSessionInput = store.CreateAgentSessionInput
type CreateAgentSendRunInput = store.CreateAgentSendRunInput
type FinalizeAgentRunInput = store.FinalizeAgentRunInput
type InterruptAgentRunInput = store.InterruptAgentRunInput
type CreateAgentModelStepRecord = core.CreateAgentModelStepRecord
type CompleteAgentModelStepRecord = core.CompleteAgentModelStepRecord
type BeginAgentToolCallRecord = core.BeginAgentToolCallRecord
type CompleteAgentToolCallRecord = core.CompleteAgentToolCallRecord
type StreamChunk = core.StreamChunk
type EventJournal = core.EventJournal
type JournalSnapshot = core.JournalSnapshot
type Service = core.Service
type ProviderMessage = core.ProviderMessage
type ProviderToolCall = core.ProviderToolCall
type ProviderFunctionCall = core.ProviderFunctionCall
type ProviderToolDefinition = core.ProviderToolDefinition
type ProviderFunctionTool = core.ProviderFunctionTool
type ModelRequest = core.ModelRequest
type ContextMessage = domain.ContextMessage
type RunExecution = domain.RunExecution
type Run = domain.Run
type RunHandle = domain.RunHandle
type AgentStep = domain.AgentStep
type MessagePart = domain.MessagePart
type Tool = core.Tool
type ToolDefinition = core.ToolDefinition
type ToolResult = core.ToolResult
type ToolResultContent = core.ToolResultContent
type ToolError = core.ToolError
type TodayReader = tools.TodayReader
type CreateItemInput = today.CreateItemInput
type Item = today.Item
type StreamProvider = core.StreamProvider
type CreateSessionRecord = core.CreateSessionRecord
type CreateSendRunRecord = core.CreateSendRunRecord
type CreateRetryRunRecord = core.CreateRetryRunRecord
type RuntimeConfig = core.RuntimeConfig
type OpenAICompatibleConfig = provider.OpenAICompatibleConfig

var OpenSQLite = store.OpenSQLite
var NewEventJournal = core.NewEventJournal
var NewAgentService = core.NewService
var NewAgentServiceWithConfig = core.NewServiceWithConfig
var NewServiceWithTools = core.NewServiceWithTools
var NewToolRegistry = core.NewToolRegistry
var NewTodayTools = tools.NewTodayTools
var NewOpenAICompatible = provider.NewOpenAICompatible
var DefaultRuntimeConfig = core.DefaultRuntimeConfig
var NewAgentError = domain.NewError
var MigrationContent = store.MigrationContent
var MigrationChecksum = store.MigrationChecksum

const (
	RunCompleted         = domain.RunCompleted
	RunRunning           = domain.RunRunning
	RunInterrupted       = domain.RunInterrupted
	MessageComplete      = domain.MessageComplete
	PartComplete         = domain.PartComplete
	PartInterrupted      = domain.PartInterrupted
	PartText             = domain.PartText
	PartReasoning        = domain.PartReasoning
	PartError            = domain.PartError
	StreamReasoningStart = core.StreamReasoningStart
	StreamReasoningDelta = core.StreamReasoningDelta
	StreamReasoningEnd   = core.StreamReasoningEnd
	StreamTextDelta      = core.StreamTextDelta
	StreamFinish         = core.StreamFinish
)

var (
	ErrSessionBusy             = core.ErrSessionBusy
	ErrRequestAlreadyProcessed = core.ErrRequestAlreadyProcessed
	ErrJournalClosed           = core.ErrJournalClosed
	ErrJournalCursorExpired    = core.ErrJournalCursorExpired
)
