package actions

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/redis"
)

// ===========================================================================
// CleanupCache — очищення протермінованих ключів кешу в Redis.
// ===========================================================================

// CleanupCache реалізує action "cleanup_cache".
// Делегує роботу до redis.Client.CleanupExpiredCache(), який сканує
// ключі за патерном "chat:*:settings" і видаляє ті, що не мають TTL.
type CleanupCache struct {
	redis  *redis.Client
	logger zerolog.Logger
}

// NewCleanupCache створює новий обробник CleanupCache.
func NewCleanupCache(redisClient *redis.Client, logger zerolog.Logger) *CleanupCache {
	return &CleanupCache{
		redis:  redisClient,
		logger: logger.With().Str("action", "cleanup_cache").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *CleanupCache) Name() string { return "cleanup_cache" }

// Execute виконує очищення кешу.
func (a *CleanupCache) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	a.logger.Debug().Msg("executing cleanup_cache")

	startedAt := time.Now().UTC()

	if err := a.redis.CleanupExpiredCache(ctx); err != nil {
		a.logger.Error().Err(err).Msg("cache cleanup failed")
		return domain.NewErrorResult("REDIS_ERROR", "cache cleanup failed"), err
	}

	duration := time.Since(startedAt)

	a.logger.Info().
		Dur("duration", duration).
		Msg("cache cleanup completed")

	return domain.NewSuccessResult(map[string]any{
		"cleaned":     true,
		"duration_ms": duration.Milliseconds(),
		"cleaned_at":  startedAt.Format(time.RFC3339),
	}), nil
}
