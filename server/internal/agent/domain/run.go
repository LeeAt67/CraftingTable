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

type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RunExecution struct {
	Run             Run
	AssistantPartID string
	Messages        []ContextMessage
}

type RunHandle struct {
	RunID              string    `json:"runId"`
	SessionID          string    `json:"sessionId"`
	UserMessageID      string    `json:"userMessageId"`
	AssistantMessageID string    `json:"assistantMessageId"`
	AssistantPartID    string    `json:"assistantPartId"`
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
