package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

const (
	idempotencySessionCreate = "session.create"
	idempotencyMessageSend   = "message.send"
	idempotencyMessageRetry  = "message.retry"
)

type CreateAgentSessionInput = core.CreateSessionRecord
type CreateAgentSendRunInput = core.CreateSendRunRecord
type CreateAgentRetryRunInput = core.CreateRetryRunRecord

type FinalizeAgentRunInput = core.FinalizeRunRecord

func (s *SQLiteStore) withTx(
	ctx context.Context,
	fn func(*sql.Tx) error,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CreateAgentSession(
	ctx context.Context,
	input core.CreateSessionRecord,
) (domain.Session, error) {
	var (
		session     domain.Session
		reusedRunID string
	)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		sessionID, runID, found, err := lookupIdempotency(
			ctx,
			tx,
			idempotencySessionCreate,
			input.ClientRequestID,
			input.RequestHash,
		)
		if err != nil {
			return err
		}
		if found {
			if sessionID == "" {
				return core.ErrIdempotencyConflict
			}
			session, err = getSessionRecord(ctx, tx, sessionID)
			reusedRunID = runID
			return err
		}

		id, err := newID()
		if err != nil {
			return fmt.Errorf("create session id: %w", err)
		}
		now := timestamp()
		session = domain.Session{
			ID:        id,
			Title:     "新会话",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_sessions (
			  id, title, client_request_id, activity_seq, created_at, updated_at
			) VALUES (?, ?, ?, 0, ?, ?)`,
			session.ID,
			session.Title,
			input.ClientRequestID,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert agent session: %w", err)
		}
		return insertIdempotency(
			ctx,
			tx,
			idempotencySessionCreate,
			input.ClientRequestID,
			input.RequestHash,
			session.ID,
			"",
		)
	})
	_ = reusedRunID
	return session, err
}

func (s *SQLiteStore) ListAgentSessions(
	ctx context.Context,
) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, title, created_at, updated_at
		 FROM agent_sessions
		 ORDER BY activity_seq DESC, updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list agent sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]domain.Session, 0)
	for rows.Next() {
		var session domain.Session
		if err := rows.Scan(
			&session.ID,
			&session.Title,
			&session.CreatedAt,
			&session.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent sessions: %w", err)
	}
	return sessions, nil
}

func (s *SQLiteStore) GetAgentSession(
	ctx context.Context,
	sessionID string,
) (domain.SessionDetail, error) {
	session, err := getSessionRecord(ctx, s.db, sessionID)
	if err != nil {
		return domain.SessionDetail{}, err
	}

	messages, err := listMessages(ctx, s.db, sessionID)
	if err != nil {
		return domain.SessionDetail{}, err
	}
	parts, err := listVisibleParts(ctx, s.db, sessionID)
	if err != nil {
		return domain.SessionDetail{}, err
	}
	activeRun, err := getActiveRun(ctx, s.db, sessionID)
	if err != nil {
		return domain.SessionDetail{}, err
	}
	return domain.SessionDetail{
		Session:           session,
		Messages:          messages,
		MessageParts:      parts,
		ActiveRun:         activeRun,
		NextMessageCursor: nil,
	}, nil
}

func (s *SQLiteStore) CreateAgentSendRun(
	ctx context.Context,
	input core.CreateSendRunRecord,
) (domain.RunExecution, error) {
	var runID string
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		existingSessionID, existingRunID, found, err := lookupIdempotency(
			ctx,
			tx,
			idempotencyMessageSend,
			input.ClientRequestID,
			input.RequestHash,
		)
		if err != nil {
			return err
		}
		if found {
			if existingSessionID != input.SessionID || existingRunID == "" {
				return core.ErrIdempotencyConflict
			}
			runID = existingRunID
			return nil
		}

		now := timestamp()
		if err := ensureSessionExists(ctx, tx, input.SessionID); err != nil {
			return err
		}
		if err := ensureSessionIdle(ctx, tx, input.SessionID); err != nil {
			return err
		}

		userSeq, err := nextMessageSequence(ctx, tx, input.SessionID)
		if err != nil {
			return err
		}
		assistantMessageID, err := newID()
		if err != nil {
			return fmt.Errorf("create assistant message id: %w", err)
		}
		runID, err = newID()
		if err != nil {
			return fmt.Errorf("create run id: %w", err)
		}
		assistantPartID, err := newID()
		if err != nil {
			return fmt.Errorf("create assistant part id: %w", err)
		}
		userText := strings.Join(input.UserParts, "\n")

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_messages (
			  id, session_id, seq, role, status, created_at, updated_at
			) VALUES (?, ?, ?, 'user', 'complete', ?, ?)`,
			input.UserMessageID,
			input.SessionID,
			userSeq,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert user message: %w", err)
		}
		for index, text := range input.UserParts {
			partID, err := newID()
			if err != nil {
				return fmt.Errorf("create user part id: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO agent_message_parts (
				  id, message_id, part_index, type, status, text_content,
				  created_at, updated_at
				) VALUES (?, ?, ?, 'text', 'complete', ?, ?, ?)`,
				partID,
				input.UserMessageID,
				index,
				text,
				now,
				now,
			); err != nil {
				return fmt.Errorf("insert user message part: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_messages (
			  id, session_id, seq, role, status, user_message_id,
			  created_at, updated_at
			) VALUES (?, ?, ?, 'assistant', 'streaming', ?, ?, ?)`,
			assistantMessageID,
			input.SessionID,
			userSeq+1,
			input.UserMessageID,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert assistant message: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_runs (
			  id, session_id, user_message_id, assistant_message_id, status,
			  client_request_id, next_event_sequence, started_at, created_at,
			  updated_at
			) VALUES (?, ?, ?, ?, 'running', ?, 1, ?, ?, ?)`,
			runID,
			input.SessionID,
			input.UserMessageID,
			assistantMessageID,
			input.ClientRequestID,
			now,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert agent run: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_message_parts (
			  id, message_id, run_id, part_index, type, status, text_content,
			  created_at, updated_at
			) VALUES (?, ?, ?, 0, 'text', 'streaming', '', ?, ?)`,
			assistantPartID,
			assistantMessageID,
			runID,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert assistant message part: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_steps (
			  id, run_id, step_index, status, created_at
			) VALUES (?, ?, 0, 'running', ?)`,
			assistantPartID+"-step-0",
			runID,
			now,
		); err != nil {
			return fmt.Errorf("insert agent step: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_messages SET active_run_id = ?, updated_at = ?
			 WHERE id = ?`,
			runID,
			now,
			assistantMessageID,
		); err != nil {
			return fmt.Errorf("activate assistant run: %w", err)
		}
		for _, eventType := range []string{
			"run.started",
			"step.started",
			"message.part.started",
		} {
			if err := appendRunEvent(ctx, tx, runID, eventType, map[string]string{
				"runId":  runID,
				"status": "running",
			}, now); err != nil {
				return err
			}
		}
		if err := insertIdempotency(
			ctx,
			tx,
			idempotencyMessageSend,
			input.ClientRequestID,
			input.RequestHash,
			input.SessionID,
			runID,
		); err != nil {
			return err
		}
		return touchSession(ctx, tx, input.SessionID, userText, now)
	})
	if err != nil {
		return domain.RunExecution{}, err
	}
	return s.getRunExecution(ctx, runID)
}

func (s *SQLiteStore) CreateAgentRetryRun(
	ctx context.Context,
	input core.CreateRetryRunRecord,
) (domain.RunExecution, error) {
	var runID string
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		existingSessionID, existingRunID, found, err := lookupIdempotency(
			ctx,
			tx,
			idempotencyMessageRetry,
			input.ClientRequestID,
			input.RequestHash,
		)
		if err != nil {
			return err
		}
		if found {
			if existingSessionID != input.SessionID || existingRunID == "" {
				return core.ErrIdempotencyConflict
			}
			runID = existingRunID
			return nil
		}

		if err := ensureSessionIdle(ctx, tx, input.SessionID); err != nil {
			return err
		}
		var (
			userMessageID string
			status        domain.MessageStatus
			role          domain.MessageRole
		)
		err = tx.QueryRowContext(
			ctx,
			`SELECT user_message_id, status, role
			 FROM agent_messages
			 WHERE id = ? AND session_id = ?`,
			input.AssistantMessageID,
			input.SessionID,
		).Scan(&userMessageID, &status, &role)
		if errors.Is(err, sql.ErrNoRows) {
			return core.ErrMessageNotRetryable
		}
		if err != nil {
			return fmt.Errorf("load retry target: %w", err)
		}
		if role != domain.MessageAssistant ||
			(status != domain.MessageFailed &&
				status != domain.MessageCancelled &&
				status != domain.MessageInterrupted) {
			return core.ErrMessageNotRetryable
		}

		runID, err = newID()
		if err != nil {
			return fmt.Errorf("create retry run id: %w", err)
		}
		partID, err := newID()
		if err != nil {
			return fmt.Errorf("create retry part id: %w", err)
		}
		now := timestamp()
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_runs (
			  id, session_id, user_message_id, assistant_message_id, status,
			  client_request_id, next_event_sequence, started_at, created_at,
			  updated_at
			) VALUES (?, ?, ?, ?, 'running', ?, 1, ?, ?, ?)`,
			runID,
			input.SessionID,
			userMessageID,
			input.AssistantMessageID,
			input.ClientRequestID,
			now,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert retry run: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_message_parts (
			  id, message_id, run_id, part_index, type, status, text_content,
			  created_at, updated_at
			) VALUES (?, ?, ?, 0, 'text', 'streaming', '', ?, ?)`,
			partID,
			input.AssistantMessageID,
			runID,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert retry part: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_steps (
			  id, run_id, step_index, status, created_at
			) VALUES (?, ?, 0, 'running', ?)`,
			partID+"-step-0",
			runID,
			now,
		); err != nil {
			return fmt.Errorf("insert retry step: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_messages
			 SET status = 'streaming', active_run_id = ?, updated_at = ?
			 WHERE id = ?`,
			runID,
			now,
			input.AssistantMessageID,
		); err != nil {
			return fmt.Errorf("activate retry run: %w", err)
		}
		for _, eventType := range []string{
			"run.started",
			"step.started",
			"message.part.started",
		} {
			if err := appendRunEvent(ctx, tx, runID, eventType, map[string]string{
				"runId":  runID,
				"status": "running",
			}, now); err != nil {
				return err
			}
		}
		if err := insertIdempotency(
			ctx,
			tx,
			idempotencyMessageRetry,
			input.ClientRequestID,
			input.RequestHash,
			input.SessionID,
			runID,
		); err != nil {
			return err
		}
		return touchSession(ctx, tx, input.SessionID, "", now)
	})
	if err != nil {
		return domain.RunExecution{}, err
	}
	return s.getRunExecution(ctx, runID)
}

func (s *SQLiteStore) FinalizeAgentRun(
	ctx context.Context,
	input core.FinalizeRunRecord,
) error {
	terminalEventType := input.TerminalEventType
	if terminalEventType == "" {
		terminalEventType = input.CompletedEventType
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var (
			sessionID          string
			assistantMessageID string
			status             domain.RunStatus
			nextSequence       int
			stepID             string
			partMessageID      string
		)
		err := tx.QueryRowContext(
			ctx,
			`SELECT session_id, assistant_message_id, status, next_event_sequence
			 FROM agent_runs WHERE id = ?`,
			input.RunID,
		).Scan(&sessionID, &assistantMessageID, &status, &nextSequence)
		if errors.Is(err, sql.ErrNoRows) {
			return core.ErrRunNotFound
		}
		if err != nil {
			return fmt.Errorf("load run for finalize: %w", err)
		}
		if status != domain.RunRunning {
			return nil
		}
		err = tx.QueryRowContext(
			ctx,
			`SELECT message_id FROM agent_message_parts WHERE id = ? AND run_id = ?`,
			input.AssistantPartID,
			input.RunID,
		).Scan(&partMessageID)
		if errors.Is(err, sql.ErrNoRows) {
			return core.ErrRunNotFound
		}
		if err != nil {
			return fmt.Errorf("load assistant part: %w", err)
		}
		if partMessageID != assistantMessageID {
			return core.ErrRunNotFound
		}

		now := timestamp()
		errorJSON, err := encodeAgentError(input.AgentError)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_message_parts
			 SET text_content = ?, status = ?, error_json = ?, updated_at = ?
			 WHERE id = ?`,
			input.AssistantText,
			input.PartStatus,
			errorJSON,
			now,
			input.AssistantPartID,
		); err != nil {
			return fmt.Errorf("finalize assistant part: %w", err)
		}
		if input.AgentError != nil {
			if err := ensureErrorPart(
				ctx,
				tx,
				assistantMessageID,
				input.RunID,
				*input.AgentError,
				now,
			); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_messages SET status = ?, updated_at = ? WHERE id = ?`,
			input.MessageStatus,
			now,
			assistantMessageID,
		); err != nil {
			return fmt.Errorf("finalize assistant message: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_runs
			 SET status = ?, error_json = ?, finished_at = ?, updated_at = ?
			 WHERE id = ?`,
			input.Status,
			errorJSON,
			now,
			now,
			input.RunID,
		); err != nil {
			return fmt.Errorf("finalize agent run: %w", err)
		}
		stepStatus := string(input.Status)
		if err := tx.QueryRowContext(
			ctx,
			`SELECT id FROM agent_steps WHERE run_id = ? AND step_index = 0`,
			input.RunID,
		).Scan(&stepID); err == nil {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE agent_steps SET status = ?, finished_at = ? WHERE id = ?`,
				stepStatus,
				now,
				stepID,
			); err != nil {
				return fmt.Errorf("finalize agent step: %w", err)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load agent step: %w", err)
		}
		if terminalEventType == "" {
			terminalEventType = terminalEventForStatus(input.Status)
		}
		if err := appendRunEventAt(
			ctx,
			tx,
			input.RunID,
			nextSequence,
			terminalEventType,
			map[string]any{
				"runId":  input.RunID,
				"status": input.Status,
			},
			now,
		); err != nil {
			return err
		}
		return touchSession(ctx, tx, sessionID, "", now)
	})
}

func (s *SQLiteStore) GetAgentRun(
	ctx context.Context,
	runID string,
) (domain.Run, error) {
	return getRunRecord(ctx, s.db, runID)
}

func (s *SQLiteStore) RecoverInterruptedAgentRuns(
	ctx context.Context,
) (int, error) {
	count := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(
			ctx,
			`SELECT id, session_id, assistant_message_id, next_event_sequence
			 FROM agent_runs WHERE status = 'running'`,
		)
		if err != nil {
			return fmt.Errorf("list running agent runs: %w", err)
		}
		type runningRun struct {
			id                 string
			sessionID          string
			assistantMessageID string
			nextSequence       int
		}
		runs := make([]runningRun, 0)
		for rows.Next() {
			var run runningRun
			if err := rows.Scan(
				&run.id,
				&run.sessionID,
				&run.assistantMessageID,
				&run.nextSequence,
			); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan running agent run: %w", err)
			}
			runs = append(runs, run)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate running agent runs: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close running agent runs: %w", err)
		}

		now := timestamp()
		for _, run := range runs {
			runError := domain.NewError(
				"AGENT_RUN_INTERRUPTED",
				"server",
				"Agent 运行在服务重启时中断",
				true,
			)
			errorJSON, err := encodeAgentError(&runError)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE agent_runs
				 SET status = 'interrupted', error_json = ?, finished_at = ?,
				     updated_at = ?
				 WHERE id = ? AND status = 'running'`,
				errorJSON,
				now,
				now,
				run.id,
			); err != nil {
				return fmt.Errorf("interrupt agent run: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE agent_messages SET status = 'interrupted', updated_at = ?
				 WHERE id = ?`,
				now,
				run.assistantMessageID,
			); err != nil {
				return fmt.Errorf("interrupt assistant message: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE agent_message_parts SET status = 'interrupted', updated_at = ?
				 WHERE run_id = ? AND status = 'streaming'`,
				now,
				run.id,
			); err != nil {
				return fmt.Errorf("interrupt assistant part: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE agent_steps SET status = 'interrupted', finished_at = ?
				 WHERE run_id = ? AND status = 'running'`,
				now,
				run.id,
			); err != nil {
				return fmt.Errorf("interrupt agent step: %w", err)
			}
			if err := appendRunEventAt(
				ctx,
				tx,
				run.id,
				run.nextSequence,
				"run.interrupted",
				map[string]any{
					"runId":  run.id,
					"status": domain.RunInterrupted,
				},
				now,
			); err != nil {
				return err
			}
			if err := touchSession(ctx, tx, run.sessionID, "", now); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (s *SQLiteStore) getRunExecution(
	ctx context.Context,
	runID string,
) (domain.RunExecution, error) {
	run, err := getRunRecord(ctx, s.db, runID)
	if err != nil {
		return domain.RunExecution{}, err
	}
	var assistantPartID string
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id FROM agent_message_parts
		 WHERE message_id = ? AND run_id = ? AND type = 'text' AND part_index = 0`,
		run.AssistantMessageID,
		run.ID,
	).Scan(&assistantPartID)
	if err != nil {
		return domain.RunExecution{}, fmt.Errorf("load run assistant part: %w", err)
	}
	detail, err := s.GetAgentSession(ctx, run.SessionID)
	if err != nil {
		return domain.RunExecution{}, err
	}
	return domain.RunExecution{
		Run:             run,
		AssistantPartID: assistantPartID,
		Messages:        contextMessages(detail.Messages, detail.MessageParts),
	}, nil
}

func getSessionRecord(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	sessionID string,
) (domain.Session, error) {
	var session domain.Session
	err := queryer.QueryRowContext(
		ctx,
		`SELECT id, title, created_at, updated_at
		 FROM agent_sessions WHERE id = ?`,
		sessionID,
	).Scan(
		&session.ID,
		&session.Title,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, core.ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("load agent session: %w", err)
	}
	return session, nil
}

func listMessages(
	ctx context.Context,
	queryer interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	sessionID string,
) ([]domain.Message, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT id, session_id, seq, role, status, active_run_id,
		        user_message_id, created_at, updated_at
		 FROM agent_messages
		 WHERE session_id = ?
		 ORDER BY seq`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agent messages: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.Message, 0)
	for rows.Next() {
		var (
			message       domain.Message
			role          string
			status        string
			activeRunID   sql.NullString
			userMessageID sql.NullString
		)
		if err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.Seq,
			&role,
			&status,
			&activeRunID,
			&userMessageID,
			&message.CreatedAt,
			&message.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent message: %w", err)
		}
		message.Role = domain.MessageRole(role)
		message.Status = domain.MessageStatus(status)
		message.ActiveRunID = stringPointer(activeRunID)
		message.UserMessageID = stringPointer(userMessageID)
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent messages: %w", err)
	}
	return messages, nil
}

func listVisibleParts(
	ctx context.Context,
	queryer interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	sessionID string,
) ([]domain.MessagePart, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT p.id, p.message_id, p.run_id, p.part_index, p.type,
		        p.status, p.text_content, p.error_json, p.created_at,
		        p.updated_at
		 FROM agent_message_parts p
		 JOIN agent_messages m ON m.id = p.message_id
		 WHERE m.session_id = ?
		   AND (
		     (m.role = 'user' AND p.run_id IS NULL)
		     OR (m.role = 'assistant' AND p.run_id = m.active_run_id)
		   )
		 ORDER BY m.seq, p.part_index`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list visible agent parts: %w", err)
	}
	defer rows.Close()

	parts := make([]domain.MessagePart, 0)
	for rows.Next() {
		part, err := scanMessagePart(rows)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visible agent parts: %w", err)
	}
	return parts, nil
}

func scanMessagePart(scanner rowScanner) (domain.MessagePart, error) {
	var (
		part      domain.MessagePart
		runID     sql.NullString
		partType  string
		status    string
		errorJSON sql.NullString
	)
	err := scanner.Scan(
		&part.ID,
		&part.MessageID,
		&runID,
		&part.Index,
		&partType,
		&status,
		&part.Text,
		&errorJSON,
		&part.CreatedAt,
		&part.UpdatedAt,
	)
	if err != nil {
		return domain.MessagePart{}, fmt.Errorf("scan agent message part: %w", err)
	}
	part.RunID = stringPointer(runID)
	part.Type = domain.PartType(partType)
	part.Status = domain.PartStatus(status)
	if errorJSON.Valid {
		var agentError domain.Error
		if err := json.Unmarshal([]byte(errorJSON.String), &agentError); err != nil {
			return domain.MessagePart{}, fmt.Errorf("decode agent part error: %w", err)
		}
		part.Error = &agentError
	}
	return part, nil
}

func getActiveRun(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	sessionID string,
) (*domain.ActiveRun, error) {
	var (
		active  domain.ActiveRun
		status  string
		nextSeq int
	)
	err := queryer.QueryRowContext(
		ctx,
		`SELECT id, user_message_id, assistant_message_id, status,
		        next_event_sequence
		 FROM agent_runs
		 WHERE session_id = ? AND status = 'running'`,
		sessionID,
	).Scan(
		&active.RunID,
		&active.UserMessageID,
		&active.AssistantMessageID,
		&status,
		&nextSeq,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load active agent run: %w", err)
	}
	active.Status = domain.RunStatus(status)
	active.LastSequence = nextSeq - 1
	return &active, nil
}

func getRunRecord(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	runID string,
) (domain.Run, error) {
	var (
		run    domain.Run
		status string
	)
	err := queryer.QueryRowContext(
		ctx,
		`SELECT id, session_id, user_message_id, assistant_message_id, status,
		        next_event_sequence, created_at, updated_at
		 FROM agent_runs WHERE id = ?`,
		runID,
	).Scan(
		&run.ID,
		&run.SessionID,
		&run.UserMessageID,
		&run.AssistantMessageID,
		&status,
		&run.NextEventSequence,
		&run.CreatedAt,
		&run.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Run{}, core.ErrRunNotFound
	}
	if err != nil {
		return domain.Run{}, fmt.Errorf("load agent run: %w", err)
	}
	run.Status = domain.RunStatus(status)
	return run, nil
}

func ensureSessionExists(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
) error {
	var exists int
	err := tx.QueryRowContext(
		ctx,
		`SELECT 1 FROM agent_sessions WHERE id = ?`,
		sessionID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("check agent session: %w", err)
	}
	return nil
}

func ensureSessionIdle(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
) error {
	var exists int
	err := tx.QueryRowContext(
		ctx,
		`SELECT 1 FROM agent_runs
		 WHERE session_id = ? AND status = 'running'`,
		sessionID,
	).Scan(&exists)
	if err == nil {
		return core.ErrSessionBusy
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check running agent run: %w", err)
	}
	return nil
}

func nextMessageSequence(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
) (int, error) {
	var maximum sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT MAX(seq) FROM agent_messages WHERE session_id = ?`,
		sessionID,
	).Scan(&maximum); err != nil {
		return 0, fmt.Errorf("load message sequence: %w", err)
	}
	if !maximum.Valid {
		return 1, nil
	}
	return int(maximum.Int64) + 1, nil
}

func lookupIdempotency(
	ctx context.Context,
	tx *sql.Tx,
	operation string,
	clientRequestID string,
	requestHash string,
) (string, string, bool, error) {
	var (
		storedHash string
		sessionID  sql.NullString
		runID      sql.NullString
	)
	err := tx.QueryRowContext(
		ctx,
		`SELECT request_hash, session_id, run_id
		 FROM agent_idempotency_keys
		 WHERE operation = ? AND client_request_id = ?`,
		operation,
		clientRequestID,
	).Scan(&storedHash, &sessionID, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load agent idempotency key: %w", err)
	}
	if storedHash != requestHash {
		return "", "", false, core.ErrIdempotencyConflict
	}
	return sessionID.String, runID.String, true, nil
}

func insertIdempotency(
	ctx context.Context,
	tx *sql.Tx,
	operation string,
	clientRequestID string,
	requestHash string,
	sessionID string,
	runID string,
) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO agent_idempotency_keys (
		  operation, client_request_id, request_hash, session_id, run_id,
		  created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		operation,
		clientRequestID,
		requestHash,
		nullableString(sessionID),
		nullableString(runID),
		timestamp(),
	)
	if err != nil {
		return fmt.Errorf("insert agent idempotency key: %w", err)
	}
	return nil
}

func appendRunEvent(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
	eventType string,
	payload any,
	now string,
) error {
	var sequence int
	err := tx.QueryRowContext(
		ctx,
		`SELECT next_event_sequence FROM agent_runs WHERE id = ?`,
		runID,
	).Scan(&sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrRunNotFound
	}
	if err != nil {
		return fmt.Errorf("load next run event sequence: %w", err)
	}
	if err := insertRunEvent(ctx, tx, runID, sequence, eventType, payload, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE agent_runs SET next_event_sequence = ? WHERE id = ?`,
		sequence+1,
		runID,
	); err != nil {
		return fmt.Errorf("advance run event sequence: %w", err)
	}
	return nil
}

func appendRunEventAt(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
	sequence int,
	eventType string,
	payload any,
	now string,
) error {
	if err := insertRunEvent(ctx, tx, runID, sequence, eventType, payload, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE agent_runs SET next_event_sequence = ? WHERE id = ?`,
		sequence+1,
		runID,
	); err != nil {
		return fmt.Errorf("advance terminal run event sequence: %w", err)
	}
	return nil
}

func insertRunEvent(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
	sequence int,
	eventType string,
	payload any,
	now string,
) error {
	id, err := newID()
	if err != nil {
		return fmt.Errorf("create run event id: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode run event payload: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO agent_run_events (
		  id, run_id, sequence, event_type, payload_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		id,
		runID,
		sequence,
		eventType,
		string(payloadJSON),
		now,
	); err != nil {
		return fmt.Errorf("insert agent run event: %w", err)
	}
	return nil
}

func touchSession(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
	titleSeed string,
	now string,
) error {
	title := truncateTitle(strings.TrimSpace(titleSeed))
	if title == "" {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_sessions
			 SET activity_seq = activity_seq + 1, updated_at = ?
			 WHERE id = ?`,
			now,
			sessionID,
		); err != nil {
			return fmt.Errorf("touch agent session: %w", err)
		}
		return nil
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE agent_sessions
		 SET title = CASE
		       WHEN activity_seq = 0 OR title = '新会话' THEN ?
		       ELSE title
		     END,
		     activity_seq = activity_seq + 1,
		     updated_at = ?
		 WHERE id = ?`,
		title,
		now,
		sessionID,
	); err != nil {
		return fmt.Errorf("update agent session activity: %w", err)
	}
	return nil
}

func truncateTitle(value string) string {
	const limit = 40
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func ensureErrorPart(
	ctx context.Context,
	tx *sql.Tx,
	messageID string,
	runID string,
	agentError domain.Error,
	now string,
) error {
	var existing int
	err := tx.QueryRowContext(
		ctx,
		`SELECT 1 FROM agent_message_parts
		 WHERE message_id = ? AND run_id = ? AND type = 'error'`,
		messageID,
		runID,
	).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check agent error part: %w", err)
	}
	var nextIndex int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(part_index), -1) + 1
		 FROM agent_message_parts WHERE message_id = ? AND run_id = ?`,
		messageID,
		runID,
	).Scan(&nextIndex); err != nil {
		return fmt.Errorf("load next error part index: %w", err)
	}
	id, err := newID()
	if err != nil {
		return fmt.Errorf("create error part id: %w", err)
	}
	errorJSON, err := encodeAgentError(&agentError)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO agent_message_parts (
		  id, message_id, run_id, part_index, type, status, text_content,
		  error_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'error', 'failed', '', ?, ?, ?)`,
		id,
		messageID,
		runID,
		nextIndex,
		errorJSON,
		now,
		now,
	); err != nil {
		return fmt.Errorf("insert agent error part: %w", err)
	}
	return nil
}

func encodeAgentError(agentError *domain.Error) (any, error) {
	if agentError == nil {
		return nil, nil
	}
	body, err := json.Marshal(agentError)
	if err != nil {
		return nil, fmt.Errorf("encode agent error: %w", err)
	}
	return string(body), nil
}

func contextMessages(
	messages []domain.Message,
	parts []domain.MessagePart,
) []domain.ContextMessage {
	result := make([]domain.ContextMessage, 0, len(messages))
	for _, message := range messages {
		var text strings.Builder
		for _, part := range parts {
			if part.MessageID == message.ID && part.Type == domain.PartText {
				text.WriteString(part.Text)
			}
		}
		if text.Len() > 0 {
			result = append(result, domain.ContextMessage{
				Role:    string(message.Role),
				Content: text.String(),
			})
		}
	}
	return result
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func terminalEventForStatus(status domain.RunStatus) string {
	switch status {
	case domain.RunCompleted:
		return "run.completed"
	case domain.RunFailed:
		return "run.failed"
	case domain.RunCancelled:
		return "run.cancelled"
	case domain.RunInterrupted:
		return "run.interrupted"
	default:
		return "run.updated"
	}
}

func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
