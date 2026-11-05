// Package testkit exposes the internal Agent persistence boundary to the
// repository's integration tests without making implementation packages public.
package testkit

import (
	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/platform/store"
)

type SQLiteStore = store.SQLiteStore

type CreateAgentSessionInput = store.CreateAgentSessionInput
type CreateAgentSendRunInput = store.CreateAgentSendRunInput
type FinalizeAgentRunInput = store.FinalizeAgentRunInput
type StreamChunk = core.StreamChunk
type EventJournal = core.EventJournal
type JournalSnapshot = core.JournalSnapshot

var OpenSQLite = store.OpenSQLite
var NewEventJournal = core.NewEventJournal

const (
	RunCompleted    = domain.RunCompleted
	RunInterrupted  = domain.RunInterrupted
	MessageComplete = domain.MessageComplete
	PartComplete    = domain.PartComplete
	PartText        = domain.PartText
	StreamTextDelta = core.StreamTextDelta
)
