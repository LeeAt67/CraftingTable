package core

import "time"

// RuntimeConfig owns execution limits that are independent of transport.
type RuntimeConfig struct {
	MaxRunDuration     time.Duration
	IdleTimeout        time.Duration
	TerminalJournalTTL time.Duration
	FinalizeTimeout    time.Duration
	FinalizeRetryDelay time.Duration
	FinalizeAttempts   int
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		MaxRunDuration:     120 * time.Second,
		IdleTimeout:        30 * time.Second,
		TerminalJournalTTL: 5 * time.Minute,
		FinalizeTimeout:    5 * time.Second,
		FinalizeRetryDelay: 100 * time.Millisecond,
		FinalizeAttempts:   3,
	}
}

func normalizedRuntimeConfig(config RuntimeConfig) RuntimeConfig {
	defaults := DefaultRuntimeConfig()
	if config.MaxRunDuration <= 0 {
		config.MaxRunDuration = defaults.MaxRunDuration
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = defaults.IdleTimeout
	}
	if config.TerminalJournalTTL <= 0 {
		config.TerminalJournalTTL = defaults.TerminalJournalTTL
	}
	if config.FinalizeTimeout <= 0 {
		config.FinalizeTimeout = defaults.FinalizeTimeout
	}
	if config.FinalizeRetryDelay <= 0 {
		config.FinalizeRetryDelay = defaults.FinalizeRetryDelay
	}
	if config.FinalizeAttempts <= 0 {
		config.FinalizeAttempts = defaults.FinalizeAttempts
	}
	return config
}
