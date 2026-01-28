// Package mysql надає GORM-обгортку для роботи з MySQL у TetraCore Worker.
// Міграції НЕ виконуються тут — ними керує Python Bot через Alembic.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/config"
)

// Client — обгортка над GORM для MySQL, з керуванням пулом з'єднань.
type Client struct {
	db     *gorm.DB
	logger zerolog.Logger
}

// NewClient створює новий MySQL-клієнт через GORM, налаштовує пул
// з'єднань та перевіряє з'єднання за допомогою Ping.
// УВАГА: AutoMigrate НЕ викликається — міграціями керує Alembic.
func NewClient(cfg config.MySQLConfig) (*Client, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("MySQL DSN не вказано")
	}

	// Вимикаємо вбудоване GORM-логування (використовуємо zerolog).
	gormCfg := &gorm.Config{
		Logger: gormlogger.Discard,
	}

	db, err := gorm.Open(gormmysql.Open(cfg.DSN), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("не вдалося підключитися до MySQL: %w", err)
	}

	// Отримуємо *sql.DB для налаштування пулу.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("не вдалося отримати *sql.DB: %w", err)
	}

	// Налаштування пулу з'єднань.
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// Перевірка з'єднання.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("не вдалося виконати ping до MySQL: %w", err)
	}

	return &Client{
		db:     db,
		logger: zerolog.Nop(),
	}, nil
}

// WithLogger встановлює логер для клієнта.
func (c *Client) WithLogger(logger zerolog.Logger) *Client {
	c.logger = logger
	return c
}

// DB повертає нативний GORM-об'єкт для виконання запитів.
func (c *Client) DB() *gorm.DB {
	return c.db
}

// Ping перевіряє зв'язок з MySQL.
func (c *Client) Ping(ctx context.Context) error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("не вдалося отримати *sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("MySQL ping: %w", err)
	}
	return nil
}

// Close закриває пул з'єднань з MySQL.
func (c *Client) Close() error {
	c.logger.Info().Msg("closing mysql connection")
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("не вдалося отримати *sql.DB для закриття: %w", err)
	}
	return sqlDB.Close()
}

// ---------------------------------------------------------------------------
// Пакетні функції для сумісності з main.go
// ---------------------------------------------------------------------------

// Close — пакетна функція для закриття MySQL-клієнта.
// Використовується як: defer mysql.Close(db)
func Close(c *Client) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		c.logger.Error().Err(err).Msg("помилка при закритті MySQL")
	}
}

// SqlDB — допоміжна функція для отримання *sql.DB з клієнта.
func (c *Client) SqlDB() (*sql.DB, error) {
	return c.db.DB()
}
