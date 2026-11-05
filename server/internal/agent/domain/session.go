package domain

type MessageRole string

const (
	MessageUser      MessageRole = "user"
	MessageAssistant MessageRole = "assistant"
)

type MessageStatus string

const (
	MessageStreaming   MessageStatus = "streaming"
	MessageComplete    MessageStatus = "complete"
	MessageFailed      MessageStatus = "failed"
	MessageCancelled   MessageStatus = "cancelled"
	MessageInterrupted MessageStatus = "interrupted"
)

type PartType string

const (
	PartText      PartType = "text"
	PartReasoning PartType = "reasoning"
	PartError     PartType = "error"
)

type PartStatus string

const (
	PartStreaming   PartStatus = "streaming"
	PartComplete    PartStatus = "complete"
	PartFailed      PartStatus = "failed"
	PartCancelled   PartStatus = "cancelled"
	PartInterrupted PartStatus = "interrupted"
)

type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Message struct {
	ID            string        `json:"id"`
	SessionID     string        `json:"sessionId"`
	Seq           int           `json:"seq"`
	Role          MessageRole   `json:"role"`
	Status        MessageStatus `json:"status"`
	ActiveRunID   *string       `json:"activeRunId,omitempty"`
	UserMessageID *string       `json:"userMessageId,omitempty"`
	CreatedAt     string        `json:"createdAt"`
	UpdatedAt     string        `json:"updatedAt"`
}

type MessagePart struct {
	ID        string     `json:"id"`
	MessageID string     `json:"messageId"`
	RunID     *string    `json:"runId,omitempty"`
	Index     int        `json:"index"`
	Type      PartType   `json:"type"`
	Status    PartStatus `json:"status"`
	Text      string     `json:"-"`
	Error     *Error     `json:"error,omitempty"`
	CreatedAt string     `json:"createdAt"`
	UpdatedAt string     `json:"updatedAt"`
}
