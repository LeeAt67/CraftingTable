package agent_test

import (
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func TestEventJournalReplaysOnlyEventsAfterCursor(t *testing.T) {
	journal := testkit.NewEventJournal(1024)
	for _, text := range []string{"你", "好"} {
		if err := journal.Append(testkit.StreamChunk{
			Kind: testkit.StreamTextDelta,
			Text: text,
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := journal.Snapshot()
	if len(snapshot.Chunks) != 2 || snapshot.LastSequence != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	events, cancel, err := journal.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	replayed := <-events
	if replayed.Sequence != 2 || replayed.Text != "好" {
		t.Fatalf("event = %#v", replayed)
	}
}

func TestEventJournalRejectsGrowthPastLimit(t *testing.T) {
	journal := testkit.NewEventJournal(200)
	if err := journal.Append(testkit.StreamChunk{
		Kind: testkit.StreamTextDelta,
		Text: string(make([]byte, 300)),
	}); err == nil {
		t.Fatal("expected journal full error")
	}
}

func TestEventJournalDropsSlowSubscriberWithoutBlocking(t *testing.T) {
	journal := testkit.NewEventJournal(4096)
	events, cancel, err := journal.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	for index := 0; index <= cap(events); index++ {
		if err := journal.Append(testkit.StreamChunk{
			Kind: testkit.StreamTextDelta,
			Text: "x",
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := journal.Snapshot()
	if snapshot.LastSequence != cap(events)+1 {
		t.Fatalf("last sequence = %d", snapshot.LastSequence)
	}
}
