package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type capturedEvent struct {
	name    string
	payload any
}

func TestRelayProviderStreamNormalizesDeltasAndDone(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"，Leeat\"}}]}\n\n" +
			"data: [DONE]\n\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].name != "delta" || events[1].name != "delta" {
		t.Fatalf("first events = %#v, want two delta events", events[:2])
	}
	if events[2].name != "done" {
		t.Fatalf("last event = %q, want done", events[2].name)
	}
}

func TestRelayProviderStreamAcceptsFinalEventWithoutBlankLine(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
			"data: [DONE]\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 2 || events[1].name != "done" {
		t.Fatalf("events = %#v, want delta then done", events)
	}
}

func TestRelayProviderStreamHonorsFinishReason(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\n",
	)
	events := make([]capturedEvent, 0)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(name string, payload any) error {
			events = append(events, capturedEvent{name: name, payload: payload})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RelayProviderStream() error = %v", err)
	}
	if len(events) != 2 || events[1].name != "done" {
		t.Fatalf("events = %#v, want delta then done", events)
	}
}

func TestRelayProviderStreamRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	source := strings.NewReader("data: not-json\n\n")
	err := RelayProviderStream(
		context.Background(),
		source,
		func(string, any) error { return nil },
	)
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error = %v, want ErrInvalidEvent", err)
	}
}

func TestRelayProviderStreamRejectsUnexpectedEOF(t *testing.T) {
	t.Parallel()

	source := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"尚未完成\"}}]}\n\n",
	)
	err := RelayProviderStream(
		context.Background(),
		source,
		func(string, any) error { return nil },
	)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want ErrUnexpectedEOF", err)
	}
}
