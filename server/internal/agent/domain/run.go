package domain

type RunStatus string

const (
	RunRunning     RunStatus = "running"
	RunCompleted   RunStatus = "completed"
	RunFailed      RunStatus = "failed"
	RunCancelled   RunStatus = "cancelled"
	RunInterrupted RunStatus = "interrupted"
)

type Run struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"sessionId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	Status             RunStatus `json:"status"`
	NextEventSequence  int       `json:"-"`
	CreatedAt          string    `json:"createdAt"`
	UpdatedAt          string    `json:"updatedAt"`
}

type RunEvent struct {
	Sequence int            `json:"sequence"`
	Type     string         `json:"type"`
	Payload  map[string]any `json:"payload"`
}

type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RunExecution struct {
	Run             Run
	ModelStepID     string
	AssistantPartID string
	ReasoningPartID string
	Messages        []ContextMessage
	Replayed        bool
}

// StepKind identifies a model or tool step.
type StepKind string

const (
	StepModel StepKind = "model"
	StepTool  StepKind = "tool"
)

// AgentStep is one durable step in a Run.
type AgentStep struct {
	ID           string
	RunID        string
	Kind         StepKind
	ParentStepID *string
	Index        int
	Status       string
	ErrorCode    *string
	CreatedAt    string
	FinishedAt   *string
}

type RunHandle struct {
	RunID              string    `json:"runId"`
	SessionID          string    `json:"sessionId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	AssistantPartID    string    `json:"assistantPartId"`
	ReasoningPartID    string    `json:"reasoningPartId,omitempty"`
	Status             RunStatus `json:"status"`
}

type ActiveRun struct {
	RunID              string    `json:"runId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	Status             RunStatus `json:"status"`
	LastSequence       int       `json:"lastSequence"`
}

type SessionDetail struct {
	Session           Session       `json:"session"`
	Messages          []Message     `json:"messages"`
	MessageParts      []MessagePart `json:"messageParts"`
	ActiveRun         *ActiveRun    `json:"activeRun"`
	NextMessageCursor *string       `json:"nextMessageCursor"`
}
