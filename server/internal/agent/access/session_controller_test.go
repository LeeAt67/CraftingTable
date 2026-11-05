package access

import (
	"encoding/json"
	"testing"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

func TestMessagePartResponsesPreserveEmptyTextContent(t *testing.T) {
	t.Parallel()

	responses := messagePartResponses([]domain.MessagePart{
		{
			ID:        "reasoning-1",
			MessageID: "message-1",
			Type:      domain.PartReasoning,
			Status:    domain.PartStreaming,
		},
		{
			ID:        "text-1",
			MessageID: "message-1",
			Type:      domain.PartText,
			Status:    domain.PartStreaming,
		},
	})

	payload, err := json.Marshal(responses)
	if err != nil {
		t.Fatalf("marshal message parts: %v", err)
	}

	var decoded []struct {
		Content map[string]any `json:"content"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal message parts: %v", err)
	}
	for index, part := range decoded {
		text, ok := part.Content["text"]
		if !ok {
			t.Errorf("part %d content is missing text: %s", index, payload)
			continue
		}
		if text != "" {
			t.Errorf("part %d text = %v, want empty string", index, text)
		}
	}
}
