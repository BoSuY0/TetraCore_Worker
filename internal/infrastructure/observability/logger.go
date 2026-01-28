// Package observability надає інструменти для логування та метрик
// TetraCore Worker: zerolog для структурованого логування,
// Prometheus для збору метрик.
package observability

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// NewLogger створює налаштований zerolog.Logger відповідно до рівня
// та формату логування.
//
// level: "trace", "debug", "info", "warn", "error", "fatal", "panic"
//
//	(за замовчуванням "info")
//
// format: "json" — стандартний JSON-вивід (для production),
//
//	"console" — людино-зрозумілий вивід (для розробки).
func NewLogger(level, format string) zerolog.Logger {
	logLevel := parseLevel(level)

	var logger zerolog.Logger

	switch strings.ToLower(format) {
	case "console":
		writer := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		logger = zerolog.New(writer).
			Level(logLevel).
			With().
			Timestamp().
			Logger()
	default:
		// JSON — production-формат.
		logger = zerolog.New(os.Stdout).
			Level(logLevel).
			With().
			Timestamp().
			Logger()
	}

	return logger
}

// WithWorkerID додає поле "worker_id" до логера.
// Повертає новий логер — оригінальний не змінюється (immutable pattern).
func WithWorkerID(logger zerolog.Logger, workerID string) zerolog.Logger {
	return logger.With().Str("worker_id", workerID).Logger()
}

// parseLevel перетворює рядкове представлення рівня логування
// у zerolog.Level. При невідомому значенні повертає zerolog.InfoLevel.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}
