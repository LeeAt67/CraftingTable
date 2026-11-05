package core

import (
	"context"
	"time"
)

func finalizeRunWithRetry(
	repository Repository,
	config RuntimeConfig,
	record FinalizeRunRecord,
) error {
	config = normalizedRuntimeConfig(config)
	var lastErr error
	for attempt := 0; attempt < config.FinalizeAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			config.FinalizeTimeout,
		)
		lastErr = repository.FinalizeAgentRun(ctx, record)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt+1 < config.FinalizeAttempts {
			time.Sleep(config.FinalizeRetryDelay)
		}
	}
	return lastErr
}

func interruptRunWithRetry(
	repository Repository,
	config RuntimeConfig,
	record InterruptRunRecord,
) error {
	config = normalizedRuntimeConfig(config)
	var lastErr error
	for attempt := 0; attempt < config.FinalizeAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			config.FinalizeTimeout,
		)
		lastErr = repository.InterruptAgentRun(ctx, record)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt+1 < config.FinalizeAttempts {
			time.Sleep(config.FinalizeRetryDelay)
		}
	}
	return lastErr
}
