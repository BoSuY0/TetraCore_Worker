package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
)

// ===========================================================================
// TestSimple — тестовий action для перевірки працездатності воркера.
// ===========================================================================

// TestSimple реалізує action "test_simple".
// Повертає ехо-відповідь з переданим повідомленням та поточним часом.
// Використовується для smoke-тестів та перевірки зв'язку Hub → Worker.
type TestSimple struct {
	logger zerolog.Logger
}

// NewTestSimple створює новий обробник TestSimple.
func NewTestSimple(logger zerolog.Logger) *TestSimple {
	return &TestSimple{
		logger: logger.With().Str("action", "test_simple").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *TestSimple) Name() string { return "test_simple" }

// Execute повертає ехо-відповідь з переданими параметрами.
func (a *TestSimple) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	a.logger.Debug().Msg("executing test_simple")

	// --- Витягуємо опціональні параметри ---
	message := "pong"
	if msgRaw, ok := params.Params["message"]; ok {
		if msgStr, ok := msgRaw.(string); ok && msgStr != "" {
			message = msgStr
		}
	}

	var timestamp string
	if tsRaw, ok := params.Params["timestamp"]; ok {
		if tsStr, ok := tsRaw.(string); ok {
			timestamp = tsStr
		}
	}

	processedAt := time.Now().UTC().Format(time.RFC3339)

	result := map[string]any{
		"success":      true,
		"message":      fmt.Sprintf("Test successful: %s", message),
		"processed_at": processedAt,
	}

	// Додаємо timestamp, якщо він був переданий.
	if timestamp != "" {
		result["received_timestamp"] = timestamp
	}

	// Додаємо task_id для трасування.
	if params.TaskID != "" {
		result["task_id"] = params.TaskID
	}

	a.logger.Info().
		Str("message", message).
		Str("processed_at", processedAt).
		Msg("test action completed")

	return domain.NewSuccessResult(result), nil
}
