package core

import (
	"testing"

	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

func TestRunStreamDoesNotEmitReasoningAfterTextStarts(t *testing.T) {
	t.Parallel()

	active := &activeRun{journal: NewEventJournal(0)}
	stream := newRunStream(active, domain.RunExecution{
		Run: domain.Run{
			ID:                 "run-1",
			SessionID:          "session-1",
			UserMessageID:      "user-1",
			AssistantMessageID: "assistant-1",
		},
		AssistantPartID: "text-1",
		ReasoningPartID: "reasoning-1",
	})

	if err := stream.reasoning("first"); err != nil {
		t.Fatal(err)
	}
	if err := stream.text("answer"); err != nil {
		t.Fatal(err)
	}
	if err := stream.reasoning("late"); err != nil {
		t.Fatal(err)
	}

	snapshot := active.journal.Snapshot()
	reasoningEnded := false
	for _, chunk := range snapshot.Chunks {
		switch chunk.Kind {
		case StreamReasoningEnd:
			reasoningEnded = true
		case StreamReasoningDelta:
			if reasoningEnded {
				t.Fatalf("reasoning delta emitted after reasoning end: %#v", chunk)
			}
		}
	}
	if snapshot.Reasoning != "first" {
		t.Fatalf("reasoning = %q, want first", snapshot.Reasoning)
	}
}
