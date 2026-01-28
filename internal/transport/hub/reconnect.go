package hub

import (
	"context"
	"fmt"
	"math"
	"time"
)

// ReconnectConfig — параметри стратегії реконнекту.
type ReconnectConfig struct {
	InitialDelay      time.Duration // Початкова затримка (за замовчуванням 5с).
	MaxDelay          time.Duration // Максимальна затримка (за замовчуванням 300с).
	Multiplier        float64       // Множник для exponential backoff (за замовчуванням 1.5).
	CircuitBreakerMax int           // Кількість послідовних невдач до паузи circuit breaker (за замовчуванням 5).
	CircuitPause      time.Duration // Тривалість паузи circuit breaker (за замовчуванням 300с).
	MaxFailures       int           // Максимум невдалих реконнектів, 0 = безліміт.
	CheckInterval     time.Duration // Інтервал перевірки з'єднання, коли підключено (за замовчуванням 30с).
}

// DefaultReconnectConfig повертає стандартну конфігурацію реконнекту.
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay:      5 * time.Second,
		MaxDelay:          300 * time.Second,
		Multiplier:        1.5,
		CircuitBreakerMax: 5,
		CircuitPause:      300 * time.Second,
		MaxFailures:       0,
		CheckInterval:     30 * time.Second,
	}
}

// ReconnectConfigFromHubConfig створює ReconnectConfig на основі HubConfig.
func ReconnectConfigFromHubConfig(cfg HubConfig, maxFailures int) ReconnectConfig {
	rc := DefaultReconnectConfig()

	if cfg.ReconnectDelay > 0 {
		rc.InitialDelay = cfg.ReconnectDelay
	}
	if cfg.MaxReconnectDelay > 0 {
		rc.MaxDelay = cfg.MaxReconnectDelay
	}
	if maxFailures > 0 {
		rc.MaxFailures = maxFailures
	}

	return rc
}

// BackgroundReconnectManager запускає горутину, що моніторить стан з'єднання
// та автоматично виконує реконнект при розриві.
//
// Логіка:
// - Коли з'єднання активне: перевірка кожні CheckInterval (30с).
// - Коли з'єднання розірване: exponential backoff від InitialDelay до MaxDelay (* Multiplier).
// - Circuit breaker: після CircuitBreakerMax послідовних невдач — пауза CircuitPause.
// - MaxFailures > 0: після N загальних невдач повертає помилку (воркер має завершитись).
//
// Повертає error лише якщо MaxFailures перевищено або контекст скасовано.
func (c *Client) BackgroundReconnectManager(ctx context.Context, rcfg ReconnectConfig) error {
	var (
		consecutiveFailures int
		totalFailures       int
		currentDelay        = rcfg.InitialDelay
	)

	c.logger.Info().
		Dur("initial_delay", rcfg.InitialDelay).
		Dur("max_delay", rcfg.MaxDelay).
		Int("circuit_breaker_max", rcfg.CircuitBreakerMax).
		Int("max_failures", rcfg.MaxFailures).
		Msg("reconnect manager started")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info().Msg("reconnect manager stopped: context cancelled")
			return ctx.Err()
		default:
		}

		if c.IsConnected() {
			// З'єднання активне — перевіряємо з фіксованим інтервалом.
			consecutiveFailures = 0
			currentDelay = rcfg.InitialDelay

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.Done():
				// З'єднання розірвано — переходимо до реконнекту.
				c.logger.Warn().Msg("connection lost, starting reconnect")
			case <-time.After(rcfg.CheckInterval):
				continue
			}
		}

		// --- Спроба реконнекту ---

		// Перевірка circuit breaker.
		if rcfg.CircuitBreakerMax > 0 && consecutiveFailures >= rcfg.CircuitBreakerMax {
			c.logger.Warn().
				Int("consecutive_failures", consecutiveFailures).
				Dur("circuit_pause", rcfg.CircuitPause).
				Msg("circuit breaker triggered, pausing")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rcfg.CircuitPause):
				// Скидаємо послідовний лічильник після паузи.
				consecutiveFailures = 0
				currentDelay = rcfg.InitialDelay
			}
		}

		// Перевірка максимального ліміту невдач.
		if rcfg.MaxFailures > 0 && totalFailures >= rcfg.MaxFailures {
			err := fmt.Errorf("max reconnect failures reached (%d)", rcfg.MaxFailures)
			c.logger.Error().Err(err).Msg("reconnect manager giving up")
			return err
		}

		// Затримка перед спробою.
		c.logger.Info().
			Int("attempt", totalFailures+1).
			Dur("delay", currentDelay).
			Msg("reconnecting after delay")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(currentDelay):
		}

		// Спроба підключення.
		if err := c.Connect(ctx); err != nil {
			consecutiveFailures++
			totalFailures++
			c.stats.ReconnectCount.Add(1)

			c.logger.Warn().
				Err(err).
				Int("consecutive", consecutiveFailures).
				Int("total", totalFailures).
				Msg("reconnect attempt failed")

			// Збільшуємо затримку (exponential backoff).
			currentDelay = time.Duration(
				math.Min(
					float64(currentDelay)*rcfg.Multiplier,
					float64(rcfg.MaxDelay),
				),
			)
			continue
		}

		// Успішне з'єднання.
		c.logger.Info().
			Int("attempts_before_success", totalFailures).
			Msg("reconnected successfully")

		consecutiveFailures = 0
		currentDelay = rcfg.InitialDelay
	}
}
