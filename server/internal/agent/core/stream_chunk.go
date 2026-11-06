package core

import "github.com/leeat/craftingtable/server/internal/agent/domain"

type StreamChunkKind string

const (
	StreamStart          StreamChunkKind = "start"
	StreamStartStep      StreamChunkKind = "start-step"
	StreamReasoningStart StreamChunkKind = "reasoning-start"
	StreamReasoningDelta StreamChunkKind = "reasoning-delta"
	StreamReasoningEnd   StreamChunkKind = "reasoning-end"
	StreamTextStart      StreamChunkKind = "text-start"
	StreamTextDelta      StreamChunkKind = "text-delta"
	StreamTextEnd        StreamChunkKind = "text-end"
	StreamFinishStep     StreamChunkKind = "finish-step"
	StreamFinish         StreamChunkKind = "finish"
	StreamError          StreamChunkKind = "error"
)

type StreamChunk struct {
	Sequence           int
	Kind               StreamChunkKind
	RunID              string
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
	PartID             string
	Text               string
	Status             domain.RunStatus
	Error              *domain.Error
}
