package core

import (
	"context"
	"errors"
	"io"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

var ErrProviderNotConfigured = errors.New("agent provider not configured")

type ProviderMessage struct {
	Role    string
	Content string
}

type StreamProvider interface {
	Stream(context.Context, []ProviderMessage) (io.ReadCloser, error)
}

func ContextMessages(messages []domain.ContextMessage) []ProviderMessage {
	result := make([]ProviderMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, ProviderMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return result
}
