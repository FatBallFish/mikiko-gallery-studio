package app

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	applogging "github.com/fatballfish/pic-gallery/internal/logging"
)

// configureDefaultLogging keeps stdout as the primary sink (Docker collects
// it) and optionally mirrors records into a size- and age-bounded file. Set
// APP_LOG_FILE on native deployments that redirect process output themselves.
func configureDefaultLogging() {
	var output io.Writer = os.Stdout
	if path := strings.TrimSpace(os.Getenv("APP_LOG_FILE")); path != "" {
		writer, err := applogging.NewRotatingWriter(applogging.Options{
			Path:       path,
			MaxBytes:   envInt64("APP_LOG_MAX_MB", 100) << 20,
			MaxBackups: envInt("APP_LOG_MAX_BACKUPS", 5),
			MaxAge:     time.Duration(envInt("APP_LOG_MAX_AGE_DAYS", 14)) * 24 * time.Hour,
		})
		if err != nil {
			log.Printf("file logging disabled: %v", err)
		} else {
			output = applogging.MultiWriter(os.Stdout, writer)
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo})))
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
