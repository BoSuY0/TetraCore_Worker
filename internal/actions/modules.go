package actions

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/mysql"
)

// ===========================================================================
// CleanupInactiveModules — деактивація модулів для неактивних чатів.
// ===========================================================================

// CleanupInactiveModules реалізує action "cleanup_inactive_modules".
// Знаходить chat_modules, які належать неактивним чатам
// (is_active = false у таблиці chats), і деактивує їх.
type CleanupInactiveModules struct {
	mysql  *mysql.Client
	logger zerolog.Logger
}

// NewCleanupInactiveModules створює новий обробник CleanupInactiveModules.
func NewCleanupInactiveModules(mysqlClient *mysql.Client, logger zerolog.Logger) *CleanupInactiveModules {
	return &CleanupInactiveModules{
		mysql:  mysqlClient,
		logger: logger.With().Str("action", "cleanup_inactive_modules").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *CleanupInactiveModules) Name() string { return "cleanup_inactive_modules" }

// Execute деактивує модулі для неактивних чатів.
func (a *CleanupInactiveModules) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	a.logger.Debug().Msg("executing cleanup_inactive_modules")

	now := time.Now().UTC()

	// Деактивуємо модулі, що належать неактивним чатам.
	// Використовуємо підзапит: chat_id IN (SELECT chat_id FROM chats WHERE is_active = false).
	result := a.mysql.DB().WithContext(ctx).
		Model(&mysql.ChatModule{}).
		Where("is_active = ? AND chat_id IN (?)",
			true,
			a.mysql.DB().Model(&mysql.Chat{}).
				Select("chat_id").
				Where("is_active = ?", false),
		).
		Updates(map[string]any{
			"is_active":      false,
			"deactivated_at": now,
		})

	if result.Error != nil {
		a.logger.Error().Err(result.Error).Msg("failed to cleanup inactive modules")
		return domain.NewErrorResult("DB_ERROR", "failed to cleanup inactive modules"), result.Error
	}

	deactivatedCount := result.RowsAffected

	a.logger.Info().
		Int64("deactivated_count", deactivatedCount).
		Time("cleaned_at", now).
		Msg("inactive modules cleaned up")

	return domain.NewSuccessResult(map[string]any{
		"deactivated_count": deactivatedCount,
		"cleaned_at":        now.Format(time.RFC3339),
	}), nil
}
