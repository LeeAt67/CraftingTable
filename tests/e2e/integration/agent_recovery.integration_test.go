package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeat/craftingtable/server/testkit"
)

type slowAgentProvider struct{}

func (slowAgentProvider) Stream(
	ctx context.Context,
	_ []testkit.ProviderMessage,
) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\n",
		); err != nil {
			return
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"第二段\"}}]}\n\n",
		); err != nil {
			return
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}()
	return reader, nil
}

type gatedAgentProvider struct {
	release <-chan struct{}
}

func (p gatedAgentProvider) Stream(
	ctx context.Context,
	_ []testkit.ProviderMessage,
) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\n",
		); err != nil {
			return
		}
		select {
		case <-p.release:
		case <-ctx.Done():
			return
		}
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"第二段\"}}]}\n\n",
		); err != nil {
			return
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}()
	return reader, nil
}

type idleAgentProvider struct{}

func (idleAgentProvider) Stream(
	ctx context.Context,
	_ []testkit.ProviderMessage,
) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if _, err := io.WriteString(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"开始\"}}]}\n\n",
		); err != nil {
			return
		}
		<-ctx.Done()
	}()
	return reader, nil
}

type flakyFinalizeRepository struct {
	*testkit.SQLiteStore
	attempts atomic.Int32
}

func (r *flakyFinalizeRepository) FinalizeAgentRun(
	ctx context.Context,
	input testkit.FinalizeAgentRunInput,
) error {
	if r.attempts.Add(1) == 1 {
		return errors.New("transient finalize failure")
	}
	return r.SQLiteStore.FinalizeAgentRun(ctx, input)
}

type failingFinalizeRepository struct {
	*testkit.SQLiteStore
	attempts atomic.Int32
}

func (r *failingFinalizeRepository) FinalizeAgentRun(
	_ context.Context,
	_ testkit.FinalizeAgentRunInput,
) error {
	r.attempts.Add(1)
	return errors.New("persistent finalize failure")
}

func (r *failingFinalizeRepository) InterruptAgentRun(
	ctx context.Context,
	input testkit.InterruptAgentRunInput,
) error {
	return r.SQLiteStore.InterruptAgentRun(ctx, input)
}

type failingPersistenceRepository struct {
	*testkit.SQLiteStore
}

func (r *failingPersistenceRepository) FinalizeAgentRun(
	_ context.Context,
	_ testkit.FinalizeAgentRunInput,
) error {
	return errors.New("finalize unavailable")
}

func (r *failingPersistenceRepository) InterruptAgentRun(
	_ context.Context,
	_ testkit.InterruptAgentRunInput,
) error {
	return errors.New("interrupt unavailable")
}

type blockingFinalizeRepository struct {
	*testkit.SQLiteStore
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
	fail    bool
}

func (r *blockingFinalizeRepository) FinalizeAgentRun(
	ctx context.Context,
	input testkit.FinalizeAgentRunInput,
) error {
	r.once.Do(func() {
		close(r.entered)
		<-r.release
	})
	if r.fail {
		return errors.New("finalize failed after completion claim")
	}
	return r.SQLiteStore.FinalizeAgentRun(ctx, input)
}

type cancelAfterCreateRepository struct {
	*testkit.SQLiteStore
	cancel context.CancelFunc
}

func (r *cancelAfterCreateRepository) CreateAgentSendRun(
	ctx context.Context,
	input testkit.CreateAgentSendRunInput,
) (testkit.RunExecution, error) {
	execution, err := r.SQLiteStore.CreateAgentSendRun(ctx, input)
	r.cancel()
	return execution, err
}

func TestCancelWhileFinalizeIsInFlightReturnsCommittedStatus(t *testing.T) {
	ctx := context.Background()
	store, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "cancel-finalize-race.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	release := make(chan struct{})
	repository := &blockingFinalizeRepository{
		SQLiteStore: store,
		entered:     make(chan struct{}),
		release:     release,
	}
	config := testkit.DefaultRuntimeConfig()
	config.FinalizeAttempts = 1
	config.FinalizeTimeout = time.Second
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		repository,
		gatedAgentProvider{release: closedSignal()},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "cancel-finalize-race-session",
		RequestHash:     "cancel-finalize-race-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "cancel-finalize-race-send",
		RequestHash:     "cancel-finalize-race-send-hash",
		UserMessageID:   "cancel-finalize-race-user",
		UserParts:       []string{"终态裁决"},
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-repository.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("finalize did not start")
	}
	type cancelResult struct {
		run testkit.Run
		err error
	}
	result := make(chan cancelResult, 1)
	go func() {
		cancelled, cancelErr := service.CancelRun(ctx, session.ID, run.RunID)
		result <- cancelResult{run: cancelled, err: cancelErr}
	}()

	select {
	case got := <-result:
		close(release)
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.run.Status != testkit.RunCompleted {
			t.Fatalf(
				"cancel returned %q while completion was already claimed, want completed",
				got.run.Status,
			)
		}
	case <-time.After(100 * time.Millisecond):
		close(release)
		got := <-result
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.run.Status != testkit.RunCompleted {
			t.Fatalf("cancel result status = %q, want completed", got.run.Status)
		}
	}
}

func TestRunStartFailureUsesDetachedFinalizeWhenRequestContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store, err := testkit.OpenSQLite(
		context.Background(),
		filepath.Join(t.TempDir(), "start-failure-cleanup.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository := &cancelAfterCreateRepository{
		SQLiteStore: store,
		cancel:      cancel,
	}
	config := testkit.DefaultRuntimeConfig()
	config.FinalizeAttempts = 2
	config.FinalizeTimeout = 500 * time.Millisecond
	config.FinalizeRetryDelay = 10 * time.Millisecond
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		repository,
		nil,
		logger,
		config,
	)
	session, err := service.CreateSession(
		context.Background(),
		testkit.CreateSessionRecord{
			ClientRequestID: "start-failure-session",
			RequestHash:     "start-failure-session-hash",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "start-failure-send",
		RequestHash:     "start-failure-send-hash",
		UserMessageID:   "start-failure-user",
		UserParts:       []string{"启动失败"},
	})
	if err == nil {
		t.Fatal("send unexpectedly started without a provider")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		detail, detailErr := store.GetAgentSession(
			context.Background(),
			session.ID,
		)
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		if len(detail.Messages) == 2 &&
			detail.Messages[1].ActiveRunID != nil {
			stored, storedErr := store.GetAgentRun(
				context.Background(),
				*detail.Messages[1].ActiveRunID,
			)
			if storedErr != nil {
				t.Fatal(storedErr)
			}
			if stored.Status == "failed" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("start-failure run remained running")
}

func TestCancelWaitingOnFailedFinalizeReportsInterrupted(t *testing.T) {
	ctx := context.Background()
	store, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "cancel-finalize-fallback.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	release := make(chan struct{})
	repository := &blockingFinalizeRepository{
		SQLiteStore: store,
		entered:     make(chan struct{}),
		release:     release,
		fail:        true,
	}
	config := testkit.DefaultRuntimeConfig()
	config.FinalizeAttempts = 1
	config.FinalizeTimeout = time.Second
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		repository,
		gatedAgentProvider{release: closedSignal()},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "cancel-finalize-fallback-session",
		RequestHash:     "cancel-finalize-fallback-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "cancel-finalize-fallback-send",
		RequestHash:     "cancel-finalize-fallback-send-hash",
		UserMessageID:   "cancel-finalize-fallback-user",
		UserParts:       []string{"终结失败"},
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-repository.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("finalize did not start")
	}
	type cancelResult struct {
		run testkit.Run
		err error
	}
	result := make(chan cancelResult, 1)
	go func() {
		cancelled, cancelErr := service.CancelRun(ctx, session.ID, run.RunID)
		result <- cancelResult{run: cancelled, err: cancelErr}
	}()
	close(release)
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.run.Status != testkit.RunInterrupted {
		t.Fatalf("cancel result status = %q, want interrupted", got.run.Status)
	}
}

func closedSignal() <-chan struct{} {
	channel := make(chan struct{})
	close(channel)
	return channel
}

func TestDuplicateSendWhileRunningDoesNotFinalizeOriginalRun(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "duplicate-running.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	release := make(chan struct{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(
		db,
		gatedAgentProvider{release: release},
		logger,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "duplicate-running-session",
		RequestHash:     "duplicate-running-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "duplicate-running-send",
		RequestHash:     "duplicate-running-send-hash",
		UserMessageID:   "duplicate-running-user",
		UserParts:       []string{"重复发送"},
	}
	first, err := service.SendMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	_, duplicateErr := service.SendMessage(ctx, input)
	if !errors.Is(duplicateErr, testkit.ErrSessionBusy) {
		t.Fatalf("duplicate error = %v, want ErrSessionBusy", duplicateErr)
	}
	run, err := db.GetAgentRun(ctx, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != testkit.RunRunning {
		t.Fatalf("duplicate send finalized original run as %q", run.Status)
	}

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err = db.GetAgentRun(ctx, first.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == testkit.RunCompleted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("original run status = %q, want completed", run.Status)
}

func TestDuplicateSendAfterTerminalReturnsProcessedError(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "duplicate-terminal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	release := make(chan struct{})
	close(release)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(
		db,
		gatedAgentProvider{release: release},
		logger,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "duplicate-terminal-session",
		RequestHash:     "duplicate-terminal-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "duplicate-terminal-send",
		RequestHash:     "duplicate-terminal-send-hash",
		UserMessageID:   "duplicate-terminal-user",
		UserParts:       []string{"终态重复发送"},
	}
	first, err := service.SendMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := db.GetAgentRun(ctx, first.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == testkit.RunCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, duplicateErr := service.SendMessage(ctx, input)
	if !errors.Is(duplicateErr, testkit.ErrRequestAlreadyProcessed) {
		t.Fatalf(
			"duplicate error = %v, want ErrRequestAlreadyProcessed",
			duplicateErr,
		)
	}
}

func TestIdleAgentProviderRunFailsWithTimeout(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "idle-timeout.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	config := testkit.DefaultRuntimeConfig()
	config.IdleTimeout = 40 * time.Millisecond
	config.MaxRunDuration = time.Second
	config.TerminalJournalTTL = time.Second
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		db,
		idleAgentProvider{},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "idle-timeout-session",
		RequestHash:     "idle-timeout-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "idle-timeout-send",
		RequestHash:     "idle-timeout-send-hash",
		UserMessageID:   "idle-timeout-user",
		UserParts:       []string{"空闲超时"},
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		storedRun, err := db.GetAgentRun(ctx, run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if storedRun.Status == "failed" {
			detail, err := db.GetAgentSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range detail.MessageParts {
				if part.Type == testkit.PartError &&
					part.Error != nil &&
					part.Error.Code == "AGENT_PROVIDER_TIMEOUT" {
					return
				}
			}
			t.Fatal("failed run did not persist AGENT_PROVIDER_TIMEOUT")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("idle run did not reach failed state")
}

func TestFinalizeAgentRunRetriesTransientPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	store, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "finalize-retry.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository := &flakyFinalizeRepository{SQLiteStore: store}

	release := make(chan struct{})
	close(release)
	config := testkit.DefaultRuntimeConfig()
	config.FinalizeAttempts = 2
	config.FinalizeTimeout = 500 * time.Millisecond
	config.FinalizeRetryDelay = 10 * time.Millisecond
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		repository,
		gatedAgentProvider{release: release},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "finalize-retry-session",
		RequestHash:     "finalize-retry-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "finalize-retry-send",
		RequestHash:     "finalize-retry-send-hash",
		UserMessageID:   "finalize-retry-user",
		UserParts:       []string{"终结重试"},
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		storedRun, err := store.GetAgentRun(ctx, run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if storedRun.Status == testkit.RunCompleted {
			if repository.attempts.Load() < 2 {
				t.Fatalf("finalize attempts = %d, want at least 2", repository.attempts.Load())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run did not complete after transient finalize failure")
}

func TestFinalizeExhaustionMarksOnlyTheAffectedRunInterrupted(t *testing.T) {
	ctx := context.Background()
	store, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "finalize-exhausted.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository := &failingFinalizeRepository{SQLiteStore: store}

	release := make(chan struct{})
	close(release)
	config := testkit.DefaultRuntimeConfig()
	config.FinalizeAttempts = 2
	config.FinalizeTimeout = 500 * time.Millisecond
	config.FinalizeRetryDelay = 10 * time.Millisecond
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		repository,
		gatedAgentProvider{release: release},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "finalize-exhausted-session",
		RequestHash:     "finalize-exhausted-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "finalize-exhausted-send",
		RequestHash:     "finalize-exhausted-send-hash",
		UserMessageID:   "finalize-exhausted-user",
		UserParts:       []string{"终结失败"},
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		storedRun, err := store.GetAgentRun(ctx, run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if storedRun.Status == testkit.RunInterrupted {
			if repository.attempts.Load() < 2 {
				t.Fatalf(
					"finalize attempts = %d, want at least 2",
					repository.attempts.Load(),
				)
			}
			return
		}
		if storedRun.Status != testkit.RunRunning {
			t.Fatalf("run status = %q, want running or interrupted", storedRun.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	storedRun, _ := store.GetAgentRun(ctx, run.RunID)
	t.Fatalf("run status = %q, want interrupted", storedRun.Status)
}

func TestJournalIsReclaimedWhenFinalizeAndInterruptBothFail(t *testing.T) {
	ctx := context.Background()
	store, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "persistence-unavailable.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository := &failingPersistenceRepository{SQLiteStore: store}

	release := make(chan struct{})
	close(release)
	config := testkit.DefaultRuntimeConfig()
	config.FinalizeAttempts = 1
	config.FinalizeTimeout = 50 * time.Millisecond
	config.TerminalJournalTTL = 20 * time.Millisecond
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		repository,
		gatedAgentProvider{release: release},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "persistence-unavailable-session",
		RequestHash:     "persistence-unavailable-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "persistence-unavailable-send",
		RequestHash:     "persistence-unavailable-send-hash",
		UserMessageID:   "persistence-unavailable-user",
		UserParts:       []string{"持久化不可用"},
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, _, subscribeErr := service.SubscribeRun(run.RunID, 0)
		if errors.Is(subscribeErr, testkit.ErrJournalCursorExpired) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("journal was not reclaimed after finalize and interrupt failed")
}

func TestTerminalRunJournalExpires(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "journal-ttl.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	release := make(chan struct{})
	close(release)
	config := testkit.DefaultRuntimeConfig()
	config.TerminalJournalTTL = 30 * time.Millisecond
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentServiceWithConfig(
		db,
		gatedAgentProvider{release: release},
		logger,
		config,
	)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "journal-ttl-session",
		RequestHash:     "journal-ttl-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "journal-ttl-send",
		RequestHash:     "journal-ttl-send-hash",
		UserMessageID:   "journal-ttl-user",
		UserParts:       []string{"journal TTL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		storedRun, err := db.GetAgentRun(ctx, run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if storedRun.Status == testkit.RunCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	_, _, err = service.SubscribeRun(run.RunID, 0)
	if !errors.Is(err, testkit.ErrJournalCursorExpired) {
		t.Fatalf("subscribe error = %v, want journal cursor expired", err)
	}
}

func TestAgentRunContinuesAfterSubscriberDisconnects(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "recovery.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, slowAgentProvider{}, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "recovery-session-request",
		RequestHash:     "recovery-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "recovery-send-request",
		RequestHash:     "recovery-send-hash",
		UserMessageID:   "recovery-user-message",
		UserParts:       []string{"恢复测试"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeRun(run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event, open := <-events:
			if !open {
				t.Fatal("subscription closed before the first delta")
			}
			if event.Kind == testkit.StreamTextDelta && event.Text == "第一段" {
				cancel()
				goto waitForCompletion
			}
		case <-deadline:
			t.Fatal("did not receive the first text delta")
		}
	}

waitForCompletion:
	completionDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(completionDeadline) {
		detail, err := service.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.ActiveRun == nil {
			for _, part := range detail.MessageParts {
				if part.RunID != nil &&
					*part.RunID == run.RunID &&
					part.Text == "第一段第二段" {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("run %s never reached completed text", run.RunID))
}

func TestAgentRunReplayRespectsCursor(t *testing.T) {
	ctx := context.Background()
	db, err := testkit.OpenSQLite(
		ctx,
		filepath.Join(t.TempDir(), "replay.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := testkit.NewAgentService(db, slowAgentProvider{}, logger)
	session, err := service.CreateSession(ctx, testkit.CreateSessionRecord{
		ClientRequestID: "replay-session",
		RequestHash:     "replay-session-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendMessage(ctx, testkit.CreateSendRunRecord{
		SessionID:       session.ID,
		ClientRequestID: "replay-send",
		RequestHash:     "replay-send-hash",
		UserMessageID:   "replay-user-message",
		UserParts:       []string{"游标测试"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, cancel, err := service.SubscribeRun(run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	var firstDelta testkit.StreamChunk
	for event := range events {
		if event.Kind == testkit.StreamTextDelta {
			firstDelta = event
			cancel()
			break
		}
	}
	if firstDelta.Sequence == 0 {
		t.Fatal("first delta was not observed")
	}

	replayed, stopReplay, err := service.SubscribeRun(
		run.RunID,
		firstDelta.Sequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stopReplay()
	for event := range replayed {
		if event.Sequence <= firstDelta.Sequence {
			t.Fatalf("replayed cursor %d after %d", event.Sequence, firstDelta.Sequence)
		}
		if strings.Contains(event.Text, "第一段") && event.Text != "第二段" {
			t.Fatalf("replayed old text: %#v", event)
		}
		if event.Kind == testkit.StreamFinish {
			return
		}
	}
	t.Fatal("replayed stream ended without a finish event")
}
