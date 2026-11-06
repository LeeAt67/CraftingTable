package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leeat/craftingtable/server/internal/agent/core"
	"github.com/leeat/craftingtable/server/internal/agent/domain"
)

// CreateAgentModelStep starts one additional model step.
func (s *SQLiteStore) CreateAgentModelStep(
	ctx context.Context,
	input core.CreateAgentModelStepRecord,
) (domain.AgentStep, error) {
	var step domain.AgentStep
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := ensureAgentRunRunning(ctx, tx, input.RunID); err != nil {
			return err
		}
		index, err := nextAgentStepIndex(ctx, tx, input.RunID)
		if err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return fmt.Errorf("create model step id: %w", err)
		}
		now := timestamp()
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_steps (
			  id, run_id, step_index, kind, status, created_at
			) VALUES (?, ?, ?, 'model', 'running', ?)`,
			id,
			input.RunID,
			index,
			now,
		); err != nil {
			return fmt.Errorf("insert model step: %w", err)
		}
		step = domain.AgentStep{
			ID:        id,
			RunID:     input.RunID,
			Kind:      domain.StepModel,
			Index:     index,
			Status:    "running",
			CreatedAt: now,
		}
		return nil
	})
	return step, err
}

// CompleteAgentModelStep marks one model step terminal.
func (s *SQLiteStore) CompleteAgentModelStep(
	ctx context.Context,
	input core.CompleteAgentModelStepRecord,
) error {
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "completed"
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(
			ctx,
			`UPDATE agent_steps
			 SET status = ?, error_code = ?, finished_at = ?
			 WHERE id = ? AND run_id = ? AND kind = 'model'`,
			status,
			nullableString(input.ErrorCode),
			timestamp(),
			input.StepID,
			input.RunID,
		)
		if err != nil {
			return fmt.Errorf("complete model step: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("complete model step rows: %w", err)
		}
		if affected != 1 {
			return core.ErrRunNotFound
		}
		return nil
	})
}

// CreateAgentToolCall creates a tool Step and invocation atomically.
func (s *SQLiteStore) CreateAgentToolCall(
	ctx context.Context,
	input core.BeginAgentToolCallRecord,
) (domain.AgentStep, error) {
	var step domain.AgentStep
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := ensureAgentRunRunning(ctx, tx, input.RunID); err != nil {
			return err
		}
		var parentKind string
		if err := tx.QueryRowContext(
			ctx,
			`SELECT kind FROM agent_steps
			 WHERE id = ? AND run_id = ?`,
			input.ParentStepID,
			input.RunID,
		).Scan(&parentKind); errors.Is(err, sql.ErrNoRows) {
			return core.ErrRunNotFound
		} else if err != nil {
			return fmt.Errorf("load parent tool step: %w", err)
		}
		if parentKind != string(domain.StepModel) {
			return fmt.Errorf("tool parent step must be a model step")
		}
		index, err := nextAgentStepIndex(ctx, tx, input.RunID)
		if err != nil {
			return err
		}
		stepID, err := newID()
		if err != nil {
			return fmt.Errorf("create tool step id: %w", err)
		}
		invocationID, err := newID()
		if err != nil {
			return fmt.Errorf("create tool invocation id: %w", err)
		}
		argumentsHash := strings.TrimSpace(input.ArgumentsHash)
		if argumentsHash == "" {
			hash := sha256.Sum256(input.Arguments)
			argumentsHash = hex.EncodeToString(hash[:])
		}
		now := timestamp()
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_steps (
			  id, run_id, step_index, kind, parent_step_id, status, created_at
			) VALUES (?, ?, ?, 'tool', ?, 'running', ?)`,
			stepID,
			input.RunID,
			index,
			input.ParentStepID,
			now,
		); err != nil {
			return fmt.Errorf("insert tool step: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_tool_invocations (
			  id, step_id, run_id, provider_call_id, tool_name, tool_version,
			  arguments_json, arguments_hash, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			invocationID,
			stepID,
			input.RunID,
			input.ProviderCallID,
			input.ToolName,
			input.ToolVersion,
			string(input.Arguments),
			argumentsHash,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert tool invocation: %w", err)
		}
		step = domain.AgentStep{
			ID:           stepID,
			RunID:        input.RunID,
			Kind:         domain.StepTool,
			ParentStepID: stringPointerFromValue(input.ParentStepID),
			Index:        index,
			Status:       "running",
			CreatedAt:    now,
		}
		return nil
	})
	return step, err
}

// CompleteAgentToolCall commits the result and terminal Step together.
func (s *SQLiteStore) CompleteAgentToolCall(
	ctx context.Context,
	input core.CompleteAgentToolCallRecord,
) error {
	resultJSON, err := json.Marshal(input.Result)
	if err != nil {
		return fmt.Errorf("encode tool result: %w", err)
	}
	encodedBytes := input.EncodedBytes
	if encodedBytes <= 0 {
		encodedBytes = len(resultJSON)
	}
	status := "completed"
	errorCode := ""
	if input.Result.IsError {
		status = "failed"
		if input.Result.Error != nil {
			errorCode = input.Result.Error.Code
		}
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(
			ctx,
			`UPDATE agent_tool_invocations
			 SET result_json = ?, result_is_error = ?, result_truncated = ?,
			     encoded_bytes = ?, updated_at = ?
			 WHERE step_id = ? AND run_id = ?`,
			string(resultJSON),
			boolInt(input.Result.IsError),
			boolInt(input.Result.Truncated),
			encodedBytes,
			timestamp(),
			input.StepID,
			input.RunID,
		)
		if err != nil {
			return fmt.Errorf("complete tool invocation: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("complete tool invocation rows: %w", err)
		}
		if affected != 1 {
			return core.ErrRunNotFound
		}
		result, err = tx.ExecContext(
			ctx,
			`UPDATE agent_steps
			 SET status = ?, error_code = ?, finished_at = ?
			 WHERE id = ? AND run_id = ? AND kind = 'tool'`,
			status,
			nullableString(errorCode),
			timestamp(),
			input.StepID,
			input.RunID,
		)
		if err != nil {
			return fmt.Errorf("complete tool step: %w", err)
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return fmt.Errorf("complete tool step rows: %w", err)
		}
		if affected != 1 {
			return core.ErrRunNotFound
		}
		return nil
	})
}

func ensureAgentRunRunning(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
) error {
	var exists int
	err := tx.QueryRowContext(
		ctx,
		`SELECT 1 FROM agent_runs WHERE id = ? AND status = 'running'`,
		runID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrRunNotFound
	}
	if err != nil {
		return fmt.Errorf("check running agent run: %w", err)
	}
	return nil
}

func nextAgentStepIndex(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
) (int, error) {
	var maximum sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT MAX(step_index) FROM agent_steps WHERE run_id = ?`,
		runID,
	).Scan(&maximum); err != nil {
		return 0, fmt.Errorf("load next agent step index: %w", err)
	}
	if !maximum.Valid {
		return 0, nil
	}
	return int(maximum.Int64) + 1, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func stringPointerFromValue(value string) *string {
	if value == "" {
		return nil
	}
	result := value
	return &result
}
