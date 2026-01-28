package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/mysql"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/redis"
)

// inactiveChatRetentionDays — кількість днів, після яких неактивні чати видаляються.
const inactiveChatRetentionDays = 30

// ===========================================================================
// SetGroupActiveStatus — перерахунок is_active для конкретного чату.
// DEPRECATED: is_active завжди перераховується, а не встановлюється вручну.
// ===========================================================================

// SetGroupActiveStatus реалізує action "set_group_active_status".
// Замість прямого встановлення is_active — перераховує його за формулою:
// is_active = (owner_user_id IS NOT NULL AND is_bot_admin = true).
type SetGroupActiveStatus struct {
	mysql  *mysql.Client
	redis  *redis.Client
	logger zerolog.Logger
}

// NewSetGroupActiveStatus створює новий обробник SetGroupActiveStatus.
func NewSetGroupActiveStatus(mysqlClient *mysql.Client, redisClient *redis.Client, logger zerolog.Logger) *SetGroupActiveStatus {
	return &SetGroupActiveStatus{
		mysql:  mysqlClient,
		redis:  redisClient,
		logger: logger.With().Str("action", "set_group_active_status").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *SetGroupActiveStatus) Name() string { return "set_group_active_status" }

// Execute перераховує is_active для вказаного чату.
func (a *SetGroupActiveStatus) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	// --- Валідація параметрів ---
	if params.ChatID == nil {
		return domain.NewErrorResult("INVALID_PARAMS", "chat_id is required"), domain.ErrInvalidParams
	}
	chatID := *params.ChatID

	a.logger.Debug().Int64("chat_id", chatID).Msg("executing set_group_active_status (deprecated: recomputing)")

	// --- Читання поточного стану чату ---
	var chat mysql.Chat
	result := a.mysql.DB().WithContext(ctx).
		Where("chat_id = ?", chatID).
		First(&chat)

	if result.Error != nil {
		if result.RowsAffected == 0 {
			return domain.NewErrorResult("CHAT_NOT_FOUND",
				fmt.Sprintf("chat %d not found", chatID)), domain.ErrChatNotFound
		}
		a.logger.Error().Err(result.Error).Int64("chat_id", chatID).Msg("mysql query failed")
		return domain.NewErrorResult("DB_ERROR", "database query failed"), result.Error
	}

	// --- Перерахунок is_active ---
	computedActive := chat.OwnerID != nil && chat.IsBotAdmin
	oldActive := chat.IsActive

	if oldActive != computedActive {
		updateResult := a.mysql.DB().WithContext(ctx).
			Model(&mysql.Chat{}).
			Where("chat_id = ?", chatID).
			Updates(map[string]any{
				"is_active":  computedActive,
				"updated_at": time.Now().UTC(),
			})

		if updateResult.Error != nil {
			a.logger.Error().Err(updateResult.Error).Int64("chat_id", chatID).Msg("failed to update is_active")
			return domain.NewErrorResult("DB_ERROR", "failed to update active status"), updateResult.Error
		}

		// Інвалідація кешу.
		cacheKey := chatSettingsCacheKey(chatID)
		if err := a.redis.Del(ctx, cacheKey); err != nil {
			a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("failed to invalidate cache")
		}
	}

	a.logger.Info().
		Int64("chat_id", chatID).
		Bool("old_active", oldActive).
		Bool("new_active", computedActive).
		Bool("changed", oldActive != computedActive).
		Msg("group active status recomputed")

	return domain.NewSuccessResult(map[string]any{
		"chat_id":     chatID,
		"is_active":   computedActive,
		"was_active":  oldActive,
		"changed":     oldActive != computedActive,
		"recomputed":  true,
		"computed_at": time.Now().UTC().Format(time.RFC3339),
	}), nil
}

// ===========================================================================
// RemoveInactiveChats — видалення давно неактивних чатів.
// ===========================================================================

// RemoveInactiveChats реалізує action "remove_inactive_chats".
// Видаляє чати, які неактивні (is_active = false) і не оновлювались
// протягом останніх 30 днів.
type RemoveInactiveChats struct {
	mysql  *mysql.Client
	logger zerolog.Logger
}

// NewRemoveInactiveChats створює новий обробник RemoveInactiveChats.
func NewRemoveInactiveChats(mysqlClient *mysql.Client, logger zerolog.Logger) *RemoveInactiveChats {
	return &RemoveInactiveChats{
		mysql:  mysqlClient,
		logger: logger.With().Str("action", "remove_inactive_chats").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *RemoveInactiveChats) Name() string { return "remove_inactive_chats" }

// Execute видаляє давно неактивні чати з бази даних.
func (a *RemoveInactiveChats) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	a.logger.Debug().Msg("executing remove_inactive_chats")

	cutoffDate := time.Now().UTC().AddDate(0, 0, -inactiveChatRetentionDays)

	result := a.mysql.DB().WithContext(ctx).
		Where("is_active = ? AND updated_at < ?", false, cutoffDate).
		Delete(&mysql.Chat{})

	if result.Error != nil {
		a.logger.Error().Err(result.Error).Msg("failed to delete inactive chats")
		return domain.NewErrorResult("DB_ERROR", "failed to delete inactive chats"), result.Error
	}

	deletedCount := result.RowsAffected

	a.logger.Info().
		Int64("deleted_count", deletedCount).
		Time("cutoff_date", cutoffDate).
		Msg("inactive chats removed")

	return domain.NewSuccessResult(map[string]any{
		"deleted_count":  deletedCount,
		"cutoff_date":    cutoffDate.Format(time.RFC3339),
		"retention_days": inactiveChatRetentionDays,
	}), nil
}

// ===========================================================================
// SetGroupStatus — перерахунок is_active (дублікат SetGroupActiveStatus).
//
// DEPRECATED: це застарілий action. Використовуйте "set_group_active_status".
//
// УВАГА: НЕ ВИДАЛЯТИ. Цей action зареєстровано у RegisterAll (registry.go),
// тому Hub може надсилати команди з action="set_group_status". Видалення
// призведе до помилки UNKNOWN_ACTION для запитів від Hub, які ще
// використовують стару назву.
//
// План міграції:
//   1. Hub поступово переходить на "set_group_active_status".
//   2. Після підтвердження, що Hub більше не надсилає "set_group_status",
//      цей action і його реєстрацію можна безпечно видалити.
// ===========================================================================

// SetGroupStatus реалізує action "set_group_status".
// Функціонально ідентичний SetGroupActiveStatus — перераховує is_active
// замість прямого встановлення.
type SetGroupStatus struct {
	mysql  *mysql.Client
	redis  *redis.Client
	logger zerolog.Logger
}

// NewSetGroupStatus створює новий обробник SetGroupStatus.
func NewSetGroupStatus(mysqlClient *mysql.Client, redisClient *redis.Client, logger zerolog.Logger) *SetGroupStatus {
	return &SetGroupStatus{
		mysql:  mysqlClient,
		redis:  redisClient,
		logger: logger.With().Str("action", "set_group_status").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *SetGroupStatus) Name() string { return "set_group_status" }

// Execute перераховує is_active для вказаного чату.
// Логіка ідентична SetGroupActiveStatus.
func (a *SetGroupStatus) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	// --- Валідація параметрів ---
	if params.ChatID == nil {
		return domain.NewErrorResult("INVALID_PARAMS", "chat_id is required"), domain.ErrInvalidParams
	}
	chatID := *params.ChatID

	a.logger.Debug().Int64("chat_id", chatID).Msg("executing set_group_status (deprecated: recomputing)")

	// --- Читання поточного стану чату ---
	var chat mysql.Chat
	result := a.mysql.DB().WithContext(ctx).
		Where("chat_id = ?", chatID).
		First(&chat)

	if result.Error != nil {
		if result.RowsAffected == 0 {
			return domain.NewErrorResult("CHAT_NOT_FOUND",
				fmt.Sprintf("chat %d not found", chatID)), domain.ErrChatNotFound
		}
		a.logger.Error().Err(result.Error).Int64("chat_id", chatID).Msg("mysql query failed")
		return domain.NewErrorResult("DB_ERROR", "database query failed"), result.Error
	}

	// --- Перерахунок is_active ---
	computedActive := chat.OwnerID != nil && chat.IsBotAdmin
	oldActive := chat.IsActive

	if oldActive != computedActive {
		updateResult := a.mysql.DB().WithContext(ctx).
			Model(&mysql.Chat{}).
			Where("chat_id = ?", chatID).
			Updates(map[string]any{
				"is_active":  computedActive,
				"updated_at": time.Now().UTC(),
			})

		if updateResult.Error != nil {
			a.logger.Error().Err(updateResult.Error).Int64("chat_id", chatID).Msg("failed to update is_active")
			return domain.NewErrorResult("DB_ERROR", "failed to update active status"), updateResult.Error
		}

		// Інвалідація кешу.
		cacheKey := chatSettingsCacheKey(chatID)
		if err := a.redis.Del(ctx, cacheKey); err != nil {
			a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("failed to invalidate cache")
		}
	}

	a.logger.Info().
		Int64("chat_id", chatID).
		Bool("old_active", oldActive).
		Bool("new_active", computedActive).
		Bool("changed", oldActive != computedActive).
		Msg("group status recomputed")

	return domain.NewSuccessResult(map[string]any{
		"chat_id":     chatID,
		"is_active":   computedActive,
		"was_active":  oldActive,
		"changed":     oldActive != computedActive,
		"recomputed":  true,
		"deprecated":  true,
		"message":     "use set_group_active_status instead",
		"computed_at": time.Now().UTC().Format(time.RFC3339),
	}), nil
}
