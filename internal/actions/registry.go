// Package actions містить реєстр та реалізації всіх action-обробників
// бізнес-логіки TetraCore Worker. Кожен action реалізує інтерфейс
// domain.ActionHandler і реєструється через Registry.
package actions

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/mysql"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/redis"
	"github.com/BoSuY0/tetracore-worker/internal/transport/telegram"
)

// Registry — потокобезпечний реєстр action-обробників.
// Дозволяє динамічно реєструвати, шукати та виконувати action-и за назвою.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]domain.ActionHandler
	logger   zerolog.Logger
}

// NewRegistry створює новий порожній реєстр action-ів.
// Логер встановлюється через WithLogger або RegisterAll.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]domain.ActionHandler),
		logger:   zerolog.Nop(),
	}
}

// WithLogger встановлює логер для реєстру.
func (r *Registry) WithLogger(logger zerolog.Logger) *Registry {
	r.logger = logger
	return r
}

// Register додає action-обробник до реєстру.
// Якщо обробник з такою назвою вже існує — він буде перезаписаний
// з попередженням у лог.
func (r *Registry) Register(handler domain.ActionHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := handler.Name()
	if _, exists := r.handlers[name]; exists {
		r.logger.Warn().
			Str("action", name).
			Msg("action handler overwritten in registry")
	}

	r.handlers[name] = handler
	r.logger.Debug().
		Str("action", name).
		Msg("action handler registered")
}

// Execute знаходить обробник за назвою action та виконує його.
// Повертає domain.ErrUnknownAction, якщо action не зареєстровано.
func (r *Registry) Execute(ctx context.Context, action string, params domain.ActionParams) (domain.ActionResult, error) {
	r.mu.RLock()
	handler, exists := r.handlers[action]
	r.mu.RUnlock()

	if !exists {
		return domain.NewErrorResult("UNKNOWN_ACTION", fmt.Sprintf("action %q not found", action)),
			domain.ErrUnknownAction
	}

	return handler.Execute(ctx, params)
}

// Get повертає обробник за назвою або nil, якщо не знайдено.
func (r *Registry) Get(action string) domain.ActionHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[action]
}

// Has перевіряє, чи зареєстровано action з вказаною назвою.
func (r *Registry) Has(action string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.handlers[action]
	return exists
}

// ListActions повертає відсортований список назв усіх зареєстрованих action-ів.
func (r *Registry) ListActions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len повертає кількість зареєстрованих action-ів.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

// RegisterAll реєструє всі вбудовані action-обробники у реєстрі.
// Приймає залежності (redis, mysql, telegram), які розподіляються
// між action-ами за принципом мінімальних привілеїв.
func RegisterAll(
	reg *Registry,
	redisClient *redis.Client,
	mysqlClient *mysql.Client,
	telegramClient *telegram.Client,
	logger zerolog.Logger,
) {
	reg.logger = logger

	// --- Chat actions ---
	reg.Register(NewGetChatSettings(redisClient, mysqlClient, logger))
	reg.Register(NewCreateGroupSettings(redisClient, mysqlClient, logger))

	// --- Maintenance actions ---
	reg.Register(NewSetGroupActiveStatus(mysqlClient, redisClient, logger))
	reg.Register(NewRemoveInactiveChats(mysqlClient, logger))
	reg.Register(NewSetGroupStatus(mysqlClient, redisClient, logger))

	// --- Subscription actions ---
	reg.Register(NewCheckSubscriptions(mysqlClient, logger))

	// --- Module actions ---
	reg.Register(NewCleanupInactiveModules(mysqlClient, logger))

	// --- Cache actions ---
	reg.Register(NewCleanupCache(redisClient, logger))

	// --- Admin actions ---
	reg.Register(NewCheckUserAdmin(telegramClient, logger))

	// --- Test actions ---
	reg.Register(NewTestSimple(logger))
}
