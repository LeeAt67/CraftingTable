package core

import (
	"errors"
	"sync"
)

var (
	ErrJournalClosed        = errors.New("agent event journal closed")
	ErrJournalFull          = errors.New("agent event journal full")
	ErrJournalCursorExpired = errors.New("agent event journal cursor expired")
)

const subscriberBuffer = 16

type JournalSnapshot struct {
	Chunks       []StreamChunk
	Text         string
	LastSequence int
}

type journalSubscriber struct {
	events chan StreamChunk
	closed bool
}

type EventJournal struct {
	mu           sync.Mutex
	maxBytes     int64
	bytes        int64
	nextSequence int
	text         []byte
	chunks       []StreamChunk
	subscribers  map[uint64]*journalSubscriber
	nextID       uint64
	closed       bool
}

func NewEventJournal(maxBytes int64) *EventJournal {
	return &EventJournal{
		maxBytes:     maxBytes,
		nextSequence: 1,
		chunks:       make([]StreamChunk, 0),
		subscribers:  make(map[uint64]*journalSubscriber),
	}
}

func (j *EventJournal) Append(chunk StreamChunk) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return ErrJournalClosed
	}
	size := int64(len(chunk.Text) + 128)
	if j.maxBytes > 0 && j.bytes+size > j.maxBytes {
		return ErrJournalFull
	}
	chunk.Sequence = j.nextSequence
	j.nextSequence++
	j.bytes += size
	j.chunks = append(j.chunks, chunk)
	if chunk.Kind == StreamTextDelta {
		j.text = append(j.text, chunk.Text...)
	}
	for id, subscriber := range j.subscribers {
		if subscriber.closed {
			delete(j.subscribers, id)
			continue
		}
		select {
		case subscriber.events <- chunk:
		default:
			j.closeSubscriberLocked(id, subscriber)
		}
	}
	return nil
}

func (j *EventJournal) Snapshot() JournalSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	chunks := make([]StreamChunk, len(j.chunks))
	copy(chunks, j.chunks)
	return JournalSnapshot{
		Chunks:       chunks,
		Text:         string(j.text),
		LastSequence: j.nextSequence - 1,
	}
}

func (j *EventJournal) Subscribe(
	after int,
) (<-chan StreamChunk, func(), error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, func() {}, ErrJournalClosed
	}
	lastSequence := j.nextSequence - 1
	if after < 0 || after > lastSequence {
		return nil, func() {}, ErrJournalCursorExpired
	}

	id := j.nextID
	j.nextID++
	subscriber := &journalSubscriber{
		events: make(chan StreamChunk, subscriberBuffer),
	}
	j.subscribers[id] = subscriber
	for _, chunk := range j.chunks {
		if chunk.Sequence <= after {
			continue
		}
		select {
		case subscriber.events <- chunk:
		default:
			j.closeSubscriberLocked(id, subscriber)
			return subscriber.events, func() {}, nil
		}
	}
	return subscriber.events, func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		if current, exists := j.subscribers[id]; exists {
			j.closeSubscriberLocked(id, current)
		}
	}, nil
}

func (j *EventJournal) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	j.closed = true
	for id, subscriber := range j.subscribers {
		j.closeSubscriberLocked(id, subscriber)
	}
}

func (j *EventJournal) closeSubscriberLocked(
	id uint64,
	subscriber *journalSubscriber,
) {
	if subscriber.closed {
		return
	}
	subscriber.closed = true
	close(subscriber.events)
	delete(j.subscribers, id)
}
