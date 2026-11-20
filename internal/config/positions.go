package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Positions struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	RequestTimeout time.Duration
	Workers        int
	QueueSize      int
	BatchSize      int
	Priority       bool
	PushURL        string
}

func parsePositions() (Positions, error) {
	var cfg Positions
	cfg.PushURL = strings.TrimSpace(os.Getenv("PUSH_HTTP_URL"))
	if cfg.PushURL == "" {
		return cfg, fmt.Errorf("PUSH_HTTP_URL is required")
	}

	for _, option := range []struct {
		name     string
		fallback string
		target   *time.Duration
	}{
		{"POSITION_CHECK_INTERVAL", "30s", &cfg.CheckInterval},
		{"POSITION_STALE_AFTER", "60s", &cfg.StaleAfter},
		{"PUSH_REQUEST_TIMEOUT", "1s", &cfg.RequestTimeout},
	} {
		value, err := time.ParseDuration(envOr(option.name, option.fallback))
		if err != nil || value <= 0 {
			return cfg, fmt.Errorf("%s must be a positive duration", option.name)
		}
		*option.target = value
	}

	var batchSize int
	for _, option := range []struct {
		name     string
		fallback string
		target   *int
	}{
		{"POSITION_WORKERS", "4", &cfg.Workers},
		{"POSITION_QUEUE_SIZE", "100", &cfg.QueueSize},
		{"POSITION_SCAN_BATCH_SIZE", "100", &batchSize},
	} {
		value, err := strconv.Atoi(envOr(option.name, option.fallback))
		if err != nil || value <= 0 {
			return cfg, fmt.Errorf("%s must be a positive integer", option.name)
		}
		*option.target = value
	}

	batched, err := boolEnv("POSITION_BATCHED_SCAN")
	if err != nil {
		return cfg, err
	}
	if batched {
		cfg.BatchSize = batchSize
	}

	cfg.Priority, err = boolEnv("POSITION_PRIORITY_QUEUE")
	return cfg, err
}

func boolEnv(name string) (bool, error) {
	value, err := strconv.ParseBool(envOr(name, "false"))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", name, err)
	}

	return value, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}

	return fallback
}
