package actions

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/mysql"
)

// ===========================================================================
// CheckSubscriptions — скидання прострочених підписок до "free".
// ===========================================================================

// CheckSubscriptions реалізує action "check_subscriptions".
// Знаходить користувачів з простроченими підписками
// (subscription_end_date < NOW() AND subscription_type != 'free')
// і скидає їх subscription_type до "free".
type CheckSubscriptions struct {
	mysql  *mysql.Client
	logger zerolog.Logger
}

// NewCheckSubscriptions створює новий обробник CheckSubscriptions.
func NewCheckSubscriptions(mysqlClient *mysql.Client, logger zerolog.Logger) *CheckSubscriptions {
	return &CheckSubscriptions{
		mysql:  mysqlClient,
		logger: logger.With().Str("action", "check_subscriptions").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *CheckSubscriptions) Name() string { return "check_subscriptions" }

// Execute знаходить та скидає прострочені підписки.
func (a *CheckSubscriptions) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	a.logger.Debug().Msg("executing check_subscriptions")

	now := time.Now().UTC()

	// Скидаємо subscription_type до "free" для всіх користувачів
	// з простроченою підпискою.
	result := a.mysql.DB().WithContext(ctx).
		Model(&mysql.User{}).
		Where("subscription_end_date < ? AND subscription_type != ?", now, string(mysql.SubscriptionFree)).
		Updates(map[string]any{
			"subscription_type": string(mysql.SubscriptionFree),
			"updated_at":        now,
		})

	if result.Error != nil {
		a.logger.Error().Err(result.Error).Msg("failed to reset expired subscriptions")
		return domain.NewErrorResult("DB_ERROR", "failed to reset expired subscriptions"), result.Error
	}

	expiredCount := result.RowsAffected

	a.logger.Info().
		Int64("expired_count", expiredCount).
		Time("checked_at", now).
		Msg("expired subscriptions reset to free")

	return domain.NewSuccessResult(map[string]any{
		"expired_count": expiredCount,
		"reset_to":      string(mysql.SubscriptionFree),
		"checked_at":    now.Format(time.RFC3339),
	}), nil
}
