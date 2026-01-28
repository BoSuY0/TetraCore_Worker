// Package redis надає обгортку над go-redis v9 для TetraCore Worker.
// Інкапсулює підключення, пул з'єднань, TLS та базові операції
// з ключами, що використовуються action-ами воркера.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	goredis "github.com/redis/go-redis/v9"

	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/config"
)

// Client — обгортка над go-redis, яка надає типізовані методи
// та структуроване логування для операцій з Redis.
type Client struct {
	rdb    *goredis.Client
	logger zerolog.Logger
}

// NewClient створює новий Redis-клієнт на основі конфігурації,
// встановлює з'єднання та перевіряє його за допомогою PING.
func NewClient(cfg config.RedisConfig) (*Client, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("не вдалося розібрати Redis URL %q: %w", cfg.URL, err)
	}

	// Перевантаження параметрів з конфігурації.
	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	if cfg.DB != 0 {
		opts.DB = cfg.DB
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}

	// TLS
	if cfg.TLSEnabled {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	rdb := goredis.NewClient(opts)

	// Перевірка з'єднання з таймаутом.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("не вдалося підключитися до Redis: %w", err)
	}

	logger := zerolog.Nop()

	return &Client{
		rdb:    rdb,
		logger: logger,
	}, nil
}

// WithLogger встановлює логер для клієнта (builder pattern).
func (c *Client) WithLogger(logger zerolog.Logger) *Client {
	c.logger = logger
	return c
}

// ---------------------------------------------------------------------------
// Базові операції
// ---------------------------------------------------------------------------

// Get повертає значення за ключем. Повертає порожній рядок і nil,
// якщо ключ не знайдено (goredis.Nil трактується як відсутність).
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == goredis.Nil {
		return "", nil
	}
	if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("redis GET failed")
		return "", fmt.Errorf("redis GET %q: %w", key, err)
	}
	return val, nil
}

// Set встановлює значення з опціональним TTL.
// Якщо ttl == 0, ключ зберігається без обмеження часу.
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("redis SET failed")
		return fmt.Errorf("redis SET %q: %w", key, err)
	}
	return nil
}

// Del видаляє один або кілька ключів.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		c.logger.Error().Err(err).Strs("keys", keys).Msg("redis DEL failed")
		return fmt.Errorf("redis DEL: %w", err)
	}
	return nil
}

// Incr атомарно збільшує значення ключа на 1.
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	val, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("redis INCR failed")
		return 0, fmt.Errorf("redis INCR %q: %w", key, err)
	}
	return val, nil
}

// Expire встановлює TTL для існуючого ключа.
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if err := c.rdb.Expire(ctx, key, ttl).Err(); err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("redis EXPIRE failed")
		return fmt.Errorf("redis EXPIRE %q: %w", key, err)
	}
	return nil
}

// SetNX встановлює значення тільки якщо ключ не існує (атомарно).
// Використовується для ідемпотентності та розподілених блокувань.
// Повертає true, якщо ключ було створено.
func (c *Client) SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	ok, err := c.rdb.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		c.logger.Error().Err(err).Str("key", key).Msg("redis SETNX failed")
		return false, fmt.Errorf("redis SETNX %q: %w", key, err)
	}
	return ok, nil
}

// ---------------------------------------------------------------------------
// Службові методи
// ---------------------------------------------------------------------------

// Ping перевіряє зв'язок з Redis.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis PING: %w", err)
	}
	return nil
}

// Close закриває з'єднання з Redis.
func (c *Client) Close() error {
	c.logger.Info().Msg("closing redis connection")
	return c.rdb.Close()
}

// CleanupExpiredCache сканує та видаляє ключі за патерном "chat:*:settings",
// що не мають TTL (або TTL вже сплив). Використовує SCAN для уникнення
// блокування Redis на великих базах.
func (c *Client) CleanupExpiredCache(ctx context.Context) error {
	var cursor uint64
	var deleted int64
	pattern := "chat:*:settings"

	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			c.logger.Error().Err(err).Msg("redis SCAN failed during cleanup")
			return fmt.Errorf("redis SCAN cleanup: %w", err)
		}

		for _, key := range keys {
			// Перевіряємо, чи ключ має TTL. Якщо TTL == -1 (без експірації)
			// або TTL == -2 (ключ не існує), видаляємо.
			ttl, err := c.rdb.TTL(ctx, key).Result()
			if err != nil {
				c.logger.Warn().Err(err).Str("key", key).Msg("failed to get TTL during cleanup")
				continue
			}

			// TTL == -1 означає ключ без терміну дії — видаляємо.
			// TTL == -2 означає ключ не існує — пропускаємо.
			if ttl == -1*time.Second {
				if err := c.rdb.Del(ctx, key).Err(); err != nil {
					c.logger.Warn().Err(err).Str("key", key).Msg("failed to delete key during cleanup")
					continue
				}
				deleted++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	c.logger.Info().Int64("deleted", deleted).Msg("cache cleanup completed")
	return nil
}

// Raw повертає нативний go-redis клієнт для складних операцій
// (pipeline, pub/sub, Lua-скрипти тощо).
func (c *Client) Raw() *goredis.Client {
	return c.rdb
}
