// Package testkit exposes the internal Agent persistence boundary to the
// repository's integration tests without making implementation packages public.
package testkit

import (
	"github.com/leeat/craftingtable/server/internal/agent/domain"
	"github.com/leeat/craftingtable/server/internal/platform/store"
)

type SQLiteStore = store.SQLiteStore

type CreateAgentSessionInput = store.CreateAgentSessionInput
type CreateAgentSendRunInput = store.CreateAgentSendRunInput
type FinalizeAgentRunInput = store.FinalizeAgentRunInput

var OpenSQLite = store.OpenSQLite

const (
	RunCompleted    = domain.RunCompleted
	RunInterrupted  = domain.RunInterrupted
	MessageComplete = domain.MessageComplete
	PartComplete    = domain.PartComplete
	PartText        = domain.PartText
)
