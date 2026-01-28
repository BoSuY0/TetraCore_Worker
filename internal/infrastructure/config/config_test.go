package config

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// TestApplyDefaults — перевірка, що для порожнього Config встановлюються
// всі очікувані значення за замовчуванням.
// ---------------------------------------------------------------------------

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	// Hub defaults
	assertStr(t, "Hub.URL", "ws://localhost:8000/ws", cfg.Hub.URL)
	assertInt(t, "Hub.PingInterval", 30, cfg.Hub.PingInterval)
	assertInt(t, "Hub.PingTimeout", 15, cfg.Hub.PingTimeout)
	assertInt(t, "Hub.ReconnectDelay", 5, cfg.Hub.ReconnectDelay)
	assertInt(t, "Hub.MaxReconnectDelay", 60, cfg.Hub.MaxReconnectDelay)
	assertInt(t, "Hub.ConnectTimeout", 15, cfg.Hub.ConnectTimeout)
	// MaxReconnectFailures: 0 — "необмежено", default не встановлюється.
	assertInt(t, "Hub.MaxReconnectFailures", 0, cfg.Hub.MaxReconnectFailures)

	// Worker defaults
	assertInt(t, "Worker.Concurrency", 10, cfg.Worker.Concurrency)
	assertInt(t, "Worker.HeartbeatInterval", 30, cfg.Worker.HeartbeatInterval)

	// Redis defaults
	assertStr(t, "Redis.URL", "redis://localhost:6379/0", cfg.Redis.URL)
	assertInt(t, "Redis.PoolSize", 20, cfg.Redis.PoolSize)

	// MySQL defaults
	assertInt(t, "MySQL.MaxOpenConns", 25, cfg.MySQL.MaxOpenConns)
	assertInt(t, "MySQL.MaxIdleConns", 10, cfg.MySQL.MaxIdleConns)
	assertInt(t, "MySQL.ConnMaxLifetime", 300, cfg.MySQL.ConnMaxLifetime)

	// Telegram defaults
	assertStr(t, "Telegram.APIURL", "https://api.telegram.org", cfg.Telegram.APIURL)
	assertInt(t, "Telegram.Timeout", 10, cfg.Telegram.Timeout)

	// Logging defaults
	assertStr(t, "Logging.Level", "info", cfg.Logging.Level)
	assertStr(t, "Logging.Format", "json", cfg.Logging.Format)

	// Metrics defaults
	assertInt(t, "Metrics.Port", 9090, cfg.Metrics.Port)
}

// ---------------------------------------------------------------------------
// TestApplyDefaults_DoesNotOverwriteExisting — якщо поля вже мають значення,
// applyDefaults не повинна їх перезаписувати.
// ---------------------------------------------------------------------------

func TestApplyDefaults_DoesNotOverwriteExisting(t *testing.T) {
	cfg := &Config{
		Hub: HubConfig{
			URL:            "ws://custom:9999/ws",
			PingInterval:   99,
			PingTimeout:    77,
			ReconnectDelay: 11,
		},
		Worker: WorkerConfig{
			Concurrency:       42,
			HeartbeatInterval: 55,
		},
		Redis: RedisConfig{
			URL:      "redis://custom:6380/1",
			PoolSize: 50,
		},
		Logging: LoggingConfig{
			Level:  "debug",
			Format: "text",
		},
	}

	applyDefaults(cfg)

	assertStr(t, "Hub.URL", "ws://custom:9999/ws", cfg.Hub.URL)
	assertInt(t, "Hub.PingInterval", 99, cfg.Hub.PingInterval)
	assertInt(t, "Hub.PingTimeout", 77, cfg.Hub.PingTimeout)
	assertInt(t, "Hub.ReconnectDelay", 11, cfg.Hub.ReconnectDelay)
	assertInt(t, "Worker.Concurrency", 42, cfg.Worker.Concurrency)
	assertInt(t, "Worker.HeartbeatInterval", 55, cfg.Worker.HeartbeatInterval)
	assertStr(t, "Redis.URL", "redis://custom:6380/1", cfg.Redis.URL)
	assertInt(t, "Redis.PoolSize", 50, cfg.Redis.PoolSize)
	assertStr(t, "Logging.Level", "debug", cfg.Logging.Level)
	assertStr(t, "Logging.Format", "text", cfg.Logging.Format)
}

// ---------------------------------------------------------------------------
// resolveTemplate — набір тестів для обробки шаблонів ${VAR:default}.
// ---------------------------------------------------------------------------

func TestResolveTemplate_EnvVar(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")

	got := resolveTemplate("${TEST_VAR:default}")
	assertStr(t, "resolveTemplate with env", "hello", got)
}

func TestResolveTemplate_Default(t *testing.T) {
	// Переконуємось, що змінна НЕ встановлена.
	t.Setenv("UNSET_VAR_FOR_TEST", "")
	os.Unsetenv("UNSET_VAR_FOR_TEST")

	got := resolveTemplate("${UNSET_VAR_FOR_TEST:fallback}")
	assertStr(t, "resolveTemplate with default", "fallback", got)
}

func TestResolveTemplate_NoDefault(t *testing.T) {
	os.Unsetenv("COMPLETELY_ABSENT_VAR")

	got := resolveTemplate("${COMPLETELY_ABSENT_VAR}")
	assertStr(t, "resolveTemplate no default", "", got)
}

func TestResolveTemplate_PlainString(t *testing.T) {
	got := resolveTemplate("plain")
	assertStr(t, "resolveTemplate plain", "plain", got)
}

// ---------------------------------------------------------------------------
// applyEnvOverrides — перевірка підстановки ENV-змінних у конфігурацію.
// ---------------------------------------------------------------------------

func TestApplyEnvOverrides(t *testing.T) {
	cfg := &Config{}

	// Hub
	t.Setenv("HUB_URL", "ws://hub:8080/ws")
	t.Setenv("AUTH_TOKEN", "secret-token")
	t.Setenv("HUB_PING_INTERVAL", "45")
	t.Setenv("HUB_PING_TIMEOUT", "20")
	t.Setenv("HUB_RECONNECT_DELAY", "8")
	t.Setenv("HUB_MAX_RECONNECT_DELAY", "120")
	t.Setenv("HUB_CONNECT_TIMEOUT", "25")
	t.Setenv("WORKER_MAX_HUB_RECONNECT_FAILURES", "5")

	// Worker
	t.Setenv("WORKER_ID", "w-123")
	t.Setenv("WORKER_CONCURRENCY", "20")
	t.Setenv("WORKER_HEARTBEAT_INTERVAL", "60")

	// Redis
	t.Setenv("REDIS_URL", "redis://redis:6380/2")
	t.Setenv("REDIS_PASSWORD", "r-pass")
	t.Setenv("REDIS_DB", "3")
	t.Setenv("REDIS_POOL_SIZE", "30")
	t.Setenv("REDIS_TLS_ENABLED", "true")

	// MySQL
	t.Setenv("MYSQL_DSN", "user:pass@tcp(db:3306)/mydb")
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "50")
	t.Setenv("MYSQL_MAX_IDLE_CONNS", "15")
	t.Setenv("MYSQL_CONN_MAX_LIFETIME", "600")

	// Telegram
	t.Setenv("BOT_TOKEN", "bot-tok")
	t.Setenv("TELEGRAM_API_URL", "https://custom-api.telegram.org")
	t.Setenv("TELEGRAM_TIMEOUT", "30")

	// Logging
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")

	// Metrics
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_PORT", "8080")

	applyEnvOverrides(cfg)

	// Hub assertions
	assertStr(t, "Hub.URL", "ws://hub:8080/ws", cfg.Hub.URL)
	assertStr(t, "Hub.AuthToken", "secret-token", cfg.Hub.AuthToken)
	assertInt(t, "Hub.PingInterval", 45, cfg.Hub.PingInterval)
	assertInt(t, "Hub.PingTimeout", 20, cfg.Hub.PingTimeout)
	assertInt(t, "Hub.ReconnectDelay", 8, cfg.Hub.ReconnectDelay)
	assertInt(t, "Hub.MaxReconnectDelay", 120, cfg.Hub.MaxReconnectDelay)
	assertInt(t, "Hub.ConnectTimeout", 25, cfg.Hub.ConnectTimeout)
	assertInt(t, "Hub.MaxReconnectFailures", 5, cfg.Hub.MaxReconnectFailures)

	// Worker assertions
	assertStr(t, "Worker.ID", "w-123", cfg.Worker.ID)
	assertInt(t, "Worker.Concurrency", 20, cfg.Worker.Concurrency)
	assertInt(t, "Worker.HeartbeatInterval", 60, cfg.Worker.HeartbeatInterval)

	// Redis assertions
	assertStr(t, "Redis.URL", "redis://redis:6380/2", cfg.Redis.URL)
	assertStr(t, "Redis.Password", "r-pass", cfg.Redis.Password)
	assertInt(t, "Redis.DB", 3, cfg.Redis.DB)
	assertInt(t, "Redis.PoolSize", 30, cfg.Redis.PoolSize)
	assertBool(t, "Redis.TLSEnabled", true, cfg.Redis.TLSEnabled)

	// MySQL assertions
	assertStr(t, "MySQL.DSN", "user:pass@tcp(db:3306)/mydb", cfg.MySQL.DSN)
	assertInt(t, "MySQL.MaxOpenConns", 50, cfg.MySQL.MaxOpenConns)
	assertInt(t, "MySQL.MaxIdleConns", 15, cfg.MySQL.MaxIdleConns)
	assertInt(t, "MySQL.ConnMaxLifetime", 600, cfg.MySQL.ConnMaxLifetime)

	// Telegram assertions
	assertStr(t, "Telegram.BotToken", "bot-tok", cfg.Telegram.BotToken)
	assertStr(t, "Telegram.APIURL", "https://custom-api.telegram.org", cfg.Telegram.APIURL)
	assertInt(t, "Telegram.Timeout", 30, cfg.Telegram.Timeout)

	// Logging assertions
	assertStr(t, "Logging.Level", "debug", cfg.Logging.Level)
	assertStr(t, "Logging.Format", "text", cfg.Logging.Format)

	// Metrics assertions
	assertBool(t, "Metrics.Enabled", true, cfg.Metrics.Enabled)
	assertInt(t, "Metrics.Port", 8080, cfg.Metrics.Port)
}

// ---------------------------------------------------------------------------
// AUTH_TOKEN_ACTIVE — перевірка, що AUTH_TOKEN_ACTIVE записується в AuthToken.
// ---------------------------------------------------------------------------

func TestApplyEnvOverrides_AuthTokenActive(t *testing.T) {
	cfg := &Config{}

	t.Setenv("AUTH_TOKEN_ACTIVE", "active-token")

	applyEnvOverrides(cfg)

	assertStr(t, "Hub.AuthToken via AUTH_TOKEN_ACTIVE", "active-token", cfg.Hub.AuthToken)
}

// ---------------------------------------------------------------------------
// AUTH_TOKEN пріоритет — AUTH_TOKEN перевіряється ПІСЛЯ AUTH_TOKEN_ACTIVE,
// отже AUTH_TOKEN має вищий пріоритет (перезаписує).
// ---------------------------------------------------------------------------

func TestApplyEnvOverrides_AuthTokenPriority(t *testing.T) {
	cfg := &Config{}

	t.Setenv("AUTH_TOKEN_ACTIVE", "active-token")
	t.Setenv("AUTH_TOKEN", "primary-token")

	applyEnvOverrides(cfg)

	assertStr(t, "Hub.AuthToken priority (AUTH_TOKEN wins)", "primary-token", cfg.Hub.AuthToken)
}

// ---------------------------------------------------------------------------
// TestLoadFromEnv — повний цикл LoadFromEnv: ENV → overrides → defaults.
// ---------------------------------------------------------------------------

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("HUB_URL", "ws://prod:8000/ws")
	t.Setenv("AUTH_TOKEN", "env-token")
	t.Setenv("WORKER_ID", "w-env-1")
	t.Setenv("WORKER_CONCURRENCY", "16")
	t.Setenv("REDIS_URL", "redis://prod-redis:6379/0")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() повернув помилку: %v", err)
	}

	// Значення з ENV
	assertStr(t, "Hub.URL", "ws://prod:8000/ws", cfg.Hub.URL)
	assertStr(t, "Hub.AuthToken", "env-token", cfg.Hub.AuthToken)
	assertStr(t, "Worker.ID", "w-env-1", cfg.Worker.ID)
	assertInt(t, "Worker.Concurrency", 16, cfg.Worker.Concurrency)
	assertStr(t, "Redis.URL", "redis://prod-redis:6379/0", cfg.Redis.URL)

	// Defaults для полів, що не були задані через ENV
	assertInt(t, "Hub.PingInterval (default)", 30, cfg.Hub.PingInterval)
	assertInt(t, "Hub.PingTimeout (default)", 15, cfg.Hub.PingTimeout)
	assertInt(t, "Hub.ReconnectDelay (default)", 5, cfg.Hub.ReconnectDelay)
	assertInt(t, "Hub.MaxReconnectDelay (default)", 60, cfg.Hub.MaxReconnectDelay)
	assertInt(t, "Hub.ConnectTimeout (default)", 15, cfg.Hub.ConnectTimeout)
	assertInt(t, "Worker.HeartbeatInterval (default)", 30, cfg.Worker.HeartbeatInterval)
	assertInt(t, "Redis.PoolSize (default)", 20, cfg.Redis.PoolSize)
	assertStr(t, "Logging.Level (default)", "info", cfg.Logging.Level)
	assertStr(t, "Logging.Format (default)", "json", cfg.Logging.Format)
	assertInt(t, "Metrics.Port (default)", 9090, cfg.Metrics.Port)
}

// ---------------------------------------------------------------------------
// TestLoad_ValidYAML — створюємо тимчасовий YAML-файл і завантажуємо через Load.
// ---------------------------------------------------------------------------

func TestLoad_ValidYAML(t *testing.T) {
	yamlContent := `
hub:
  url: "ws://yaml-hub:9000/ws"
  auth_token: "yaml-token"
  ping_interval: 10
  ping_timeout: 5
  reconnect_delay: 3
  max_reconnect_delay: 30
  connect_timeout: 8
  max_reconnect_failures: 10
worker:
  id: "yaml-worker"
  concurrency: 8
  heartbeat_interval: 15
redis:
  url: "redis://yaml-redis:6379/1"
  password: "yaml-pass"
  db: 2
  pool_size: 40
  tls_enabled: true
mysql:
  dsn: "yaml-user:pass@tcp(yaml-db:3306)/yamldb"
  max_open_conns: 30
  max_idle_conns: 5
  conn_max_lifetime: 120
telegram:
  bot_token: "yaml-bot"
  api_url: "https://yaml-api.telegram.org"
  timeout: 20
logging:
  level: "warn"
  format: "text"
metrics:
  enabled: true
  port: 7070
`

	tmpFile, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("не вдалося створити тимчасовий файл: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("не вдалося записати YAML: %v", err)
	}
	// Закриваємо файл перед читанням через Viper.
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load(%q) повернув помилку: %v", tmpFile.Name(), err)
	}

	// Hub
	assertStr(t, "Hub.URL", "ws://yaml-hub:9000/ws", cfg.Hub.URL)
	assertStr(t, "Hub.AuthToken", "yaml-token", cfg.Hub.AuthToken)
	assertInt(t, "Hub.PingInterval", 10, cfg.Hub.PingInterval)
	assertInt(t, "Hub.PingTimeout", 5, cfg.Hub.PingTimeout)
	assertInt(t, "Hub.ReconnectDelay", 3, cfg.Hub.ReconnectDelay)
	assertInt(t, "Hub.MaxReconnectDelay", 30, cfg.Hub.MaxReconnectDelay)
	assertInt(t, "Hub.ConnectTimeout", 8, cfg.Hub.ConnectTimeout)
	assertInt(t, "Hub.MaxReconnectFailures", 10, cfg.Hub.MaxReconnectFailures)

	// Worker
	assertStr(t, "Worker.ID", "yaml-worker", cfg.Worker.ID)
	assertInt(t, "Worker.Concurrency", 8, cfg.Worker.Concurrency)
	assertInt(t, "Worker.HeartbeatInterval", 15, cfg.Worker.HeartbeatInterval)

	// Redis
	assertStr(t, "Redis.URL", "redis://yaml-redis:6379/1", cfg.Redis.URL)
	assertStr(t, "Redis.Password", "yaml-pass", cfg.Redis.Password)
	assertInt(t, "Redis.DB", 2, cfg.Redis.DB)
	assertInt(t, "Redis.PoolSize", 40, cfg.Redis.PoolSize)
	assertBool(t, "Redis.TLSEnabled", true, cfg.Redis.TLSEnabled)

	// MySQL
	assertStr(t, "MySQL.DSN", "yaml-user:pass@tcp(yaml-db:3306)/yamldb", cfg.MySQL.DSN)
	assertInt(t, "MySQL.MaxOpenConns", 30, cfg.MySQL.MaxOpenConns)
	assertInt(t, "MySQL.MaxIdleConns", 5, cfg.MySQL.MaxIdleConns)
	assertInt(t, "MySQL.ConnMaxLifetime", 120, cfg.MySQL.ConnMaxLifetime)

	// Telegram
	assertStr(t, "Telegram.BotToken", "yaml-bot", cfg.Telegram.BotToken)
	assertStr(t, "Telegram.APIURL", "https://yaml-api.telegram.org", cfg.Telegram.APIURL)
	assertInt(t, "Telegram.Timeout", 20, cfg.Telegram.Timeout)

	// Logging
	assertStr(t, "Logging.Level", "warn", cfg.Logging.Level)
	assertStr(t, "Logging.Format", "text", cfg.Logging.Format)

	// Metrics
	assertBool(t, "Metrics.Enabled", true, cfg.Metrics.Enabled)
	assertInt(t, "Metrics.Port", 7070, cfg.Metrics.Port)
}

// ---------------------------------------------------------------------------
// Допоміжні функції перевірки (helpers)
// ---------------------------------------------------------------------------

func assertStr(t *testing.T, field, want, got string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: очікувалось %q, отримано %q", field, want, got)
	}
}

func assertInt(t *testing.T, field string, want, got int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: очікувалось %d, отримано %d", field, want, got)
	}
}

func assertBool(t *testing.T, field string, want, got bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s: очікувалось %v, отримано %v", field, want, got)
	}
}
