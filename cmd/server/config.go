package main

import (
	"os"
	"strconv"
	"time"
)

type config struct {
	AudiobookDataDir string
	DatabasePath     string
	AppPort          string
	AuthUsername     string
	AuthPassword     string
	ScanInterval     time.Duration
	BaseURL          string
}

func loadConfig() config {
	scanMins := envOr("SCAN_INTERVAL_MINUTES", "60")
	mins, err := strconv.Atoi(scanMins)
	if err != nil || mins < 1 {
		mins = 60
	}

	return config{
		AudiobookDataDir: envOr("AUDIOBOOK_DATA_DIR", "/data/audiobooks"),
		DatabasePath:     envOr("DATABASE_PATH", "/data/shelfstone.db"),
		AppPort:          envOr("APP_PORT", "8080"),
		AuthUsername:     envOr("AUTH_USERNAME", ""),
		AuthPassword:     envOr("AUTH_PASSWORD", ""),
		ScanInterval:     time.Duration(mins) * time.Minute,
		BaseURL:          envOr("BASE_URL", ""),
	}
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
