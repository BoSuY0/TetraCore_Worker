package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/mysql"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/redis"
)

// ---------------------------------------------------------------------------
// Константи
// ---------------------------------------------------------------------------

const (
	// chatSettingsCacheTTL — час життя кешу налаштувань чату в Redis.
	chatSettingsCacheTTL = 5 * time.Minute

	// chatSettingsCachePrefix — префікс ключа кешу налаштувань чату.
	chatSettingsCachePrefix = "chat:"
	chatSettingsCacheSuffix = ":settings"

	// maxTitleLength — максимальна довжина назви чату.
	maxTitleLength = 128
)

// allowedLanguages — список дозволених мов для чату.
var allowedLanguages = map[string]bool{
	"uk": true,
	"en": true,
	"ru": true,
	"be": true,
	"kk": true,
}

// chatSettingsCacheKey формує ключ кешу для налаштувань чату.
func chatSettingsCacheKey(chatID int64) string {
	return fmt.Sprintf("%s%d%s", chatSettingsCachePrefix, chatID, chatSettingsCacheSuffix)
}

// ===========================================================================
// GetChatSettings — отримання налаштувань чату з кешем Redis → MySQL fallback.
// ===========================================================================

// GetChatSettings реалізує action "get_chat_settings".
// Спершу шукає дані в Redis-кеші, за відсутності — читає з MySQL
// та кешує результат на 5 хвилин.
type GetChatSettings struct {
	redis  *redis.Client
	mysql  *mysql.Client
	logger zerolog.Logger
}

// NewGetChatSettings створює новий обробник GetChatSettings.
func NewGetChatSettings(redisClient *redis.Client, mysqlClient *mysql.Client, logger zerolog.Logger) *GetChatSettings {
	return &GetChatSettings{
		redis:  redisClient,
		mysql:  mysqlClient,
		logger: logger.With().Str("action", "get_chat_settings").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *GetChatSettings) Name() string { return "get_chat_settings" }

// Execute виконує отримання налаштувань чату.
func (a *GetChatSettings) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	// --- Валідація параметрів ---
	if params.ChatID == nil {
		return domain.NewErrorResult("INVALID_PARAMS", "chat_id is required"), domain.ErrInvalidParams
	}
	chatID := *params.ChatID

	a.logger.Debug().Int64("chat_id", chatID).Msg("executing get_chat_settings")

	// --- Крок 1: спроба прочитати з Redis-кешу ---
	cacheKey := chatSettingsCacheKey(chatID)
	cached, err := a.redis.Get(ctx, cacheKey)
	if err != nil {
		// Помилка Redis не є фатальною — переходимо до MySQL.
		a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("redis cache read failed, falling back to mysql")
	}

	if cached != "" {
		// Розпаковуємо кешовані дані.
		var data map[string]any
		if err := json.Unmarshal([]byte(cached), &data); err != nil {
			a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("invalid cache data, falling back to mysql")
		} else {
			a.logger.Debug().Int64("chat_id", chatID).Msg("cache hit")
			return domain.NewSuccessResult(data), nil
		}
	}

	// --- Крок 2: читання з MySQL ---
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

	// --- Крок 3: отримання підписки через власника ---
	ownerSubscription := string(mysql.SubscriptionFree)
	if chat.OwnerID != nil {
		var owner mysql.User
		ownerResult := a.mysql.DB().WithContext(ctx).
			Where("user_id = ?", *chat.OwnerID).
			First(&owner)
		if ownerResult.Error == nil && owner.SubscriptionType != "" {
			ownerSubscription = owner.SubscriptionType
		}
	}

	// --- Крок 4: формування відповіді (тільки безпечні поля) ---
	data := mysql.ChatToMap(&chat, ownerSubscription)

	// --- Крок 5: збереження в Redis-кеш ---
	cacheBytes, err := json.Marshal(data)
	if err != nil {
		a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("failed to marshal cache data")
	} else {
		if err := a.redis.Set(ctx, cacheKey, string(cacheBytes), chatSettingsCacheTTL); err != nil {
			a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("failed to write cache")
		}
	}

	a.logger.Debug().Int64("chat_id", chatID).Msg("cache miss, loaded from mysql")
	return domain.NewSuccessResult(data), nil
}

// ===========================================================================
// CreateGroupSettings — створення або оновлення налаштувань групи (upsert).
// ===========================================================================

// CreateGroupSettings реалізує action "create_group_settings".
// Виконує upsert чату в MySQL: INSERT або UPDATE за chat_id.
type CreateGroupSettings struct {
	redis  *redis.Client
	mysql  *mysql.Client
	logger zerolog.Logger
}

// NewCreateGroupSettings створює новий обробник CreateGroupSettings.
func NewCreateGroupSettings(redisClient *redis.Client, mysqlClient *mysql.Client, logger zerolog.Logger) *CreateGroupSettings {
	return &CreateGroupSettings{
		redis:  redisClient,
		mysql:  mysqlClient,
		logger: logger.With().Str("action", "create_group_settings").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *CreateGroupSettings) Name() string { return "create_group_settings" }

// Execute виконує створення/оновлення налаштувань групи.
func (a *CreateGroupSettings) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	// --- Валідація параметрів ---
	if params.ChatID == nil {
		return domain.NewErrorResult("INVALID_PARAMS", "chat_id is required"), domain.ErrInvalidParams
	}
	chatID := *params.ChatID

	a.logger.Debug().Int64("chat_id", chatID).Msg("executing create_group_settings")

	// --- Збір оновлюваних полів ---
	updates := map[string]any{}

	// Title: очищення та обрізка.
	if titleRaw, ok := params.Params["title"]; ok {
		if titleStr, ok := titleRaw.(string); ok {
			cleaned := sanitizeTitle(titleStr)
			updates["title"] = cleaned
		}
	}

	// Language: валідація проти allowedLanguages.
	language := "uk" // default
	if langRaw, ok := params.Params["language"]; ok {
		if langStr, ok := langRaw.(string); ok {
			langStr = strings.TrimSpace(strings.ToLower(langStr))
			if allowedLanguages[langStr] {
				language = langStr
			}
		}
	}
	updates["language"] = language

	// ChatType.
	if ctRaw, ok := params.Params["chat_type"]; ok {
		if ctStr, ok := ctRaw.(string); ok {
			ctStr = strings.TrimSpace(ctStr)
			if ctStr != "" {
				updates["chat_type"] = ctStr
			}
		}
	}

	// IsForum.
	if forumRaw, ok := params.Params["is_forum"]; ok {
		if forumBool, ok := forumRaw.(bool); ok {
			updates["is_forum"] = forumBool
		}
	}

	// IsActive (може бути вказано явно, але перевизначається логікою нижче).
	if activeRaw, ok := params.Params["is_active"]; ok {
		if activeBool, ok := activeRaw.(bool); ok {
			updates["is_active"] = activeBool
		}
	}

	// IsBotAdmin.
	if adminRaw, ok := params.Params["is_bot_admin"]; ok {
		if adminBool, ok := adminRaw.(bool); ok {
			updates["is_bot_admin"] = adminBool
		}
	}

	// OwnerID.
	if ownerRaw, ok := params.Params["owner_user_id"]; ok {
		if ownerID, ok := extractInt64FromAny(ownerRaw); ok {
			updates["owner_user_id"] = ownerID
		}
	}

	// MemberCount.
	if mcRaw, ok := params.Params["member_count"]; ok {
		if mc, ok := extractInt64FromAny(mcRaw); ok {
			updates["member_count"] = int(mc)
		}
	}

	// --- Upsert через GORM ---
	now := time.Now().UTC()
	updates["updated_at"] = now

	// Defaults для нових записів.
	chat := mysql.Chat{
		ChatID:   chatID,
		ChatType: "unknown",
		Language: language,
	}

	// Якщо chat_type вказаний у updates, використовуємо його для нового запису.
	if ct, ok := updates["chat_type"].(string); ok {
		chat.ChatType = ct
	}

	result := a.mysql.DB().WithContext(ctx).
		Where("chat_id = ?", chatID).
		Assign(updates).
		FirstOrCreate(&chat)

	if result.Error != nil {
		a.logger.Error().Err(result.Error).Int64("chat_id", chatID).Msg("upsert failed")
		return domain.NewErrorResult("DB_ERROR", "failed to create/update group settings"), result.Error
	}

	// --- Перерахунок is_active: owner_id IS NOT NULL AND is_bot_admin ---
	computedActive := chat.OwnerID != nil && chat.IsBotAdmin
	if chat.IsActive != computedActive {
		a.mysql.DB().WithContext(ctx).
			Model(&mysql.Chat{}).
			Where("chat_id = ?", chatID).
			Update("is_active", computedActive)
		chat.IsActive = computedActive
	}

	// --- Інвалідація Redis-кешу ---
	cacheKey := chatSettingsCacheKey(chatID)
	if err := a.redis.Del(ctx, cacheKey); err != nil {
		a.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("failed to invalidate cache")
	}

	a.logger.Info().
		Int64("chat_id", chatID).
		Bool("created", result.RowsAffected > 0).
		Bool("is_active", chat.IsActive).
		Msg("group settings saved")

	return domain.NewSuccessResult(map[string]any{
		"chat_id":    chatID,
		"is_active":  chat.IsActive,
		"language":   chat.Language,
		"created":    result.RowsAffected > 0,
		"updated_at": now.Format(time.RFC3339),
	}), nil
}

// ===========================================================================
// Допоміжні функції
// ===========================================================================

// sanitizeTitle очищує назву чату від керуючих символів та обрізає до maxTitleLength.
func sanitizeTitle(title string) string {
	// Заміна \x00 на порожній рядок, \r\n\t на пробіл.
	var b strings.Builder
	b.Grow(len(title))

	for _, r := range title {
		switch {
		case r == 0:
			// Видаляємо null-байти.
			continue
		case r == '\r' || r == '\n' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
			// Видаляємо інші керуючі символи.
			continue
		default:
			b.WriteRune(r)
		}
	}

	result := strings.TrimSpace(b.String())

	// Обрізка до maxTitleLength символів (не байтів).
	runes := []rune(result)
	if len(runes) > maxTitleLength {
		runes = runes[:maxTitleLength]
		result = string(runes)
	}

	return result
}

// extractInt64FromAny безпечно витягує int64 з будь-якого типу.
// JSON-числа зазвичай декодуються як float64.
func extractInt64FromAny(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case int64:
		return val, true
	case int:
		return int64(val), true
	case json.Number:
		n, err := val.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}
