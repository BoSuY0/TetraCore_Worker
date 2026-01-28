package domain

import "errors"

var (
	// ErrUnknownAction — action name not found in the registry.
	ErrUnknownAction = errors.New("unknown action")
	// ErrTaskTimeout — task exceeded its allowed execution time.
	ErrTaskTimeout = errors.New("task execution timeout")
	// ErrHubDisconnected — WebSocket connection to Hub is down.
	ErrHubDisconnected = errors.New("hub is disconnected")
	// ErrHubRegistration — initial registration handshake with Hub failed.
	ErrHubRegistration = errors.New("hub registration failed")
	// ErrRedisUnavailable — Redis connection is not available.
	ErrRedisUnavailable = errors.New("redis is unavailable")
	// ErrDBUnavailable — database connection is not available.
	ErrDBUnavailable = errors.New("database is unavailable")
	// ErrTelegramAPI — Telegram Bot API returned an error.
	ErrTelegramAPI = errors.New("telegram API error")
	// ErrInvalidParams — action received invalid or missing parameters.
	ErrInvalidParams = errors.New("invalid action parameters")
	// ErrChatNotFound — requested chat does not exist.
	ErrChatNotFound = errors.New("chat not found")
	// ErrUserNotFound — requested user does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrModuleNotFound — requested module does not exist.
	ErrModuleNotFound = errors.New("module not found")
	// ErrActionDisabled — action exists but is currently disabled.
	ErrActionDisabled = errors.New("action is disabled")
	// ErrRateLimited — operation was rejected due to rate limiting.
	ErrRateLimited = errors.New("rate limited")
	// ErrWorkerShuttingDown — worker is in graceful shutdown, rejecting new tasks.
	ErrWorkerShuttingDown = errors.New("worker is shutting down")
)
