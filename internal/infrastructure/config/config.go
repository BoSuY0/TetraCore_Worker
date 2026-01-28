// Package config забезпечує завантаження та валідацію конфігурації воркера.
// Підтримує YAML-файли з підстановкою змінних середовища через Viper,
// а також завантаження конфігурації виключно з ENV-змінних.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// Основна структура конфігурації
// ---------------------------------------------------------------------------

// Config — кореневий об'єкт конфігурації воркера.
type Config struct {
	Hub      HubConfig      `mapstructure:"hub"`
	Worker   WorkerConfig   `mapstructure:"worker"`
	Redis    RedisConfig    `mapstructure:"redis"`
	MySQL    MySQLConfig    `mapstructure:"mysql"`
	Telegram TelegramConfig `mapstructure:"telegram"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
}

// HubConfig — параметри підключення до Hub через WebSocket.
type HubConfig struct {
	URL                  string `mapstructure:"url"`
	AuthToken            string `mapstructure:"auth_token"`
	PingInterval         int    `mapstructure:"ping_interval"`
	PingTimeout          int    `mapstructure:"ping_timeout"`
	ReconnectDelay       int    `mapstructure:"reconnect_delay"`
	MaxReconnectDelay    int    `mapstructure:"max_reconnect_delay"`
	ConnectTimeout       int    `mapstructure:"connect_timeout"`
	MaxReconnectFailures int    `mapstructure:"max_reconnect_failures"`
}

// WorkerConfig — параметри самого воркера.
type WorkerConfig struct {
	ID                string `mapstructure:"id"`
	Concurrency       int    `mapstructure:"concurrency"`
	HeartbeatInterval int    `mapstructure:"heartbeat_interval"`
}

// RedisConfig — параметри підключення до Redis.
type RedisConfig struct {
	URL        string `mapstructure:"url"`
	Password   string `mapstructure:"password"`
	DB         int    `mapstructure:"db"`
	PoolSize   int    `mapstructure:"pool_size"`
	TLSEnabled bool   `mapstructure:"tls_enabled"`
}

// MySQLConfig — параметри підключення до MySQL через GORM.
type MySQLConfig struct {
	DSN             string `mapstructure:"dsn"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

// TelegramConfig — параметри взаємодії з Telegram Bot API.
type TelegramConfig struct {
	BotToken string `mapstructure:"bot_token"`
	APIURL   string `mapstructure:"api_url"`
	Timeout  int    `mapstructure:"timeout"`
}

// LoggingConfig — параметри логування.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// MetricsConfig — параметри Prometheus-метрик.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}

// ---------------------------------------------------------------------------
// Завантаження з YAML + ENV
// ---------------------------------------------------------------------------

// Load завантажує конфігурацію з YAML-файлу за вказаним шляхом,
// потім перезаписує значення зі змінних середовища.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("не вдалося прочитати конфігураційний файл %q: %w", configPath, err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("не вдалося розібрати конфігурацію: %w", err)
	}

	// Viper YAML шаблони ${VAR:default} не розгортаються автоматично —
	// очищаємо «шаблонні» значення, що могли залишитись нерозгорнутими.
	cleanTemplateValues(cfg)

	// Перевантаження значень з ENV (ENV має найвищий пріоритет).
	applyEnvOverrides(cfg)

	// Встановлюємо defaults для полів, що лишились нульовими.
	applyDefaults(cfg)

	return cfg, nil
}

// ---------------------------------------------------------------------------
// Завантаження виключно з ENV
// ---------------------------------------------------------------------------

// LoadFromEnv завантажує конфігурацію тільки зі змінних середовища,
// без YAML-файлу. Використовується, коли конфіг-файл недоступний
// (напр. у Docker-контейнері з ENV-only підходом).
func LoadFromEnv() (*Config, error) {
	cfg := &Config{}
	applyEnvOverrides(cfg)
	applyDefaults(cfg)
	return cfg, nil
}

// ---------------------------------------------------------------------------
// Підстановка ENV поверх YAML
// ---------------------------------------------------------------------------

// applyEnvOverrides перезаписує поля конфігурації значеннями змінних
// середовища, якщо ці змінні встановлені.
func applyEnvOverrides(cfg *Config) {
	// Hub
	if v := os.Getenv("HUB_URL"); v != "" {
		cfg.Hub.URL = v
	}
	// AUTH_TOKEN_ACTIVE має пріоритет (сумісність з Python worker).
	if v := os.Getenv("AUTH_TOKEN_ACTIVE"); v != "" {
		cfg.Hub.AuthToken = v
	}
	if v := os.Getenv("AUTH_TOKEN"); v != "" {
		cfg.Hub.AuthToken = v
	}
	if v, ok := envInt("HUB_PING_INTERVAL"); ok {
		cfg.Hub.PingInterval = v
	}
	if v, ok := envInt("HUB_PING_TIMEOUT"); ok {
		cfg.Hub.PingTimeout = v
	}
	if v, ok := envInt("HUB_RECONNECT_DELAY"); ok {
		cfg.Hub.ReconnectDelay = v
	}
	if v, ok := envInt("HUB_MAX_RECONNECT_DELAY"); ok {
		cfg.Hub.MaxReconnectDelay = v
	}
	if v, ok := envInt("HUB_CONNECT_TIMEOUT"); ok {
		cfg.Hub.ConnectTimeout = v
	}
	if v, ok := envInt("WORKER_MAX_HUB_RECONNECT_FAILURES"); ok {
		cfg.Hub.MaxReconnectFailures = v
	}

	// Worker
	if v := os.Getenv("WORKER_ID"); v != "" {
		cfg.Worker.ID = v
	}
	if v, ok := envInt("WORKER_CONCURRENCY"); ok {
		cfg.Worker.Concurrency = v
	}
	if v, ok := envInt("WORKER_HEARTBEAT_INTERVAL"); ok {
		cfg.Worker.HeartbeatInterval = v
	}

	// Redis
	if v := os.Getenv("REDIS_URL"); v != "" {
		cfg.Redis.URL = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v, ok := envInt("REDIS_DB"); ok {
		cfg.Redis.DB = v
	}
	if v, ok := envInt("REDIS_POOL_SIZE"); ok {
		cfg.Redis.PoolSize = v
	}
	if v, ok := envBool("REDIS_TLS_ENABLED"); ok {
		cfg.Redis.TLSEnabled = v
	}

	// MySQL
	if v := os.Getenv("MYSQL_DSN"); v != "" {
		cfg.MySQL.DSN = v
	}
	if v, ok := envInt("MYSQL_MAX_OPEN_CONNS"); ok {
		cfg.MySQL.MaxOpenConns = v
	}
	if v, ok := envInt("MYSQL_MAX_IDLE_CONNS"); ok {
		cfg.MySQL.MaxIdleConns = v
	}
	if v, ok := envInt("MYSQL_CONN_MAX_LIFETIME"); ok {
		cfg.MySQL.ConnMaxLifetime = v
	}

	// Telegram
	if v := os.Getenv("BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("TELEGRAM_API_URL"); v != "" {
		cfg.Telegram.APIURL = v
	}
	if v, ok := envInt("TELEGRAM_TIMEOUT"); ok {
		cfg.Telegram.Timeout = v
	}

	// Logging
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}

	// Metrics
	if v, ok := envBool("METRICS_ENABLED"); ok {
		cfg.Metrics.Enabled = v
	}
	if v, ok := envInt("METRICS_PORT"); ok {
		cfg.Metrics.Port = v
	}
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// applyDefaults встановлює розумні значення за замовчуванням для полів,
// що лишились нульовими після YAML+ENV.
func applyDefaults(cfg *Config) {
	// Hub
	if cfg.Hub.URL == "" {
		cfg.Hub.URL = "ws://localhost:8000/ws"
	}
	if cfg.Hub.PingInterval == 0 {
		cfg.Hub.PingInterval = 30
	}
	if cfg.Hub.PingTimeout == 0 {
		cfg.Hub.PingTimeout = 15
	}
	if cfg.Hub.ReconnectDelay == 0 {
		cfg.Hub.ReconnectDelay = 5
	}
	if cfg.Hub.MaxReconnectDelay == 0 {
		cfg.Hub.MaxReconnectDelay = 60
	}
	if cfg.Hub.ConnectTimeout == 0 {
		cfg.Hub.ConnectTimeout = 15
	}
	// MaxReconnectFailures: 0 означає "необмежено" — не ставимо default.

	// Worker
	if cfg.Worker.Concurrency == 0 {
		cfg.Worker.Concurrency = 10
	}
	if cfg.Worker.HeartbeatInterval == 0 {
		cfg.Worker.HeartbeatInterval = 30
	}

	// Redis
	if cfg.Redis.URL == "" {
		cfg.Redis.URL = "redis://localhost:6379/0"
	}
	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 20
	}

	// MySQL
	if cfg.MySQL.MaxOpenConns == 0 {
		cfg.MySQL.MaxOpenConns = 25
	}
	if cfg.MySQL.MaxIdleConns == 0 {
		cfg.MySQL.MaxIdleConns = 10
	}
	if cfg.MySQL.ConnMaxLifetime == 0 {
		cfg.MySQL.ConnMaxLifetime = 300
	}

	// Telegram
	if cfg.Telegram.APIURL == "" {
		cfg.Telegram.APIURL = "https://api.telegram.org"
	}
	if cfg.Telegram.Timeout == 0 {
		cfg.Telegram.Timeout = 10
	}

	// Logging
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	// Metrics
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = 9090
	}
}

// ---------------------------------------------------------------------------
// Утиліти: робота з ENV
// ---------------------------------------------------------------------------

// envInt зчитує змінну середовища як int.
func envInt(key string) (int, bool) {
	v := os.Getenv(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// envBool зчитує змінну середовища як bool.
func envBool(key string) (bool, bool) {
	v := os.Getenv(key)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false
	}
	return b, true
}

// ---------------------------------------------------------------------------
// Очищення YAML-шаблонів
// ---------------------------------------------------------------------------

// cleanTemplateValues видаляє нерозгорнуті шаблони виду "${VAR:default}"
// та замінює їх на default-значення (або порожній рядок).
func cleanTemplateValues(cfg *Config) {
	cfg.Hub.URL = resolveTemplate(cfg.Hub.URL)
	cfg.Hub.AuthToken = resolveTemplate(cfg.Hub.AuthToken)
	cfg.Worker.ID = resolveTemplate(cfg.Worker.ID)
	cfg.Redis.URL = resolveTemplate(cfg.Redis.URL)
	cfg.Redis.Password = resolveTemplate(cfg.Redis.Password)
	cfg.MySQL.DSN = resolveTemplate(cfg.MySQL.DSN)
	cfg.Telegram.BotToken = resolveTemplate(cfg.Telegram.BotToken)
	cfg.Logging.Level = resolveTemplate(cfg.Logging.Level)
	cfg.Logging.Format = resolveTemplate(cfg.Logging.Format)
}

// resolveTemplate обробляє значення виду "${VAR:default}":
// - Якщо ENV-змінна VAR встановлена — повертає її значення.
// - Якщо ні — повертає default-частину.
// - Якщо рядок не є шаблоном — повертає його як є.
func resolveTemplate(val string) string {
	if !strings.HasPrefix(val, "${") || !strings.HasSuffix(val, "}") {
		return val
	}

	// Прибираємо ${ та }
	inner := val[2 : len(val)-1]

	// Розділяємо VAR:default
	parts := strings.SplitN(inner, ":", 2)
	envKey := parts[0]

	envVal := os.Getenv(envKey)
	if envVal != "" {
		return envVal
	}

	// Повертаємо default, якщо він є.
	if len(parts) == 2 {
		return parts[1]
	}

	return ""
}
