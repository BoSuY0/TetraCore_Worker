package actions

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/transport/telegram"
)

// ===========================================================================
// CheckUserAdmin — перевірка, чи є користувач адміністратором чату.
// ===========================================================================

// CheckUserAdmin реалізує action "check_user_admin".
// Звертається до Telegram Bot API (getChatMember) та перевіряє статус
// користувача у вказаному чаті.
type CheckUserAdmin struct {
	tg     *telegram.Client
	logger zerolog.Logger
}

// NewCheckUserAdmin створює новий обробник CheckUserAdmin.
func NewCheckUserAdmin(telegramClient *telegram.Client, logger zerolog.Logger) *CheckUserAdmin {
	return &CheckUserAdmin{
		tg:     telegramClient,
		logger: logger.With().Str("action", "check_user_admin").Logger(),
	}
}

// Name повертає ідентифікатор action-у.
func (a *CheckUserAdmin) Name() string { return "check_user_admin" }

// Execute перевіряє адміністраторський статус користувача у чаті.
func (a *CheckUserAdmin) Execute(ctx context.Context, params domain.ActionParams) (domain.ActionResult, error) {
	// --- Валідація параметрів ---
	if params.ChatID == nil {
		return domain.NewErrorResult("INVALID_PARAMS", "chat_id is required"), domain.ErrInvalidParams
	}
	if params.UserID == nil {
		return domain.NewErrorResult("INVALID_PARAMS", "user_id is required"), domain.ErrInvalidParams
	}

	chatID := *params.ChatID
	userID := *params.UserID

	a.logger.Debug().
		Int64("chat_id", chatID).
		Int64("user_id", userID).
		Msg("executing check_user_admin")

	// --- Запит до Telegram API ---
	member, err := a.tg.GetChatMember(ctx, chatID, userID)
	if err != nil {
		// Помилка Telegram API не є фатальною — повертаємо is_admin: false.
		a.logger.Warn().Err(err).
			Int64("chat_id", chatID).
			Int64("user_id", userID).
			Msg("telegram API error, defaulting is_admin to false")

		return domain.NewSuccessResult(map[string]any{
			"is_admin":  false,
			"chat_id":   chatID,
			"user_id":   userID,
			"tg_error":  true,
			"error_msg": err.Error(),
		}), nil
	}

	isAdmin := telegram.IsAdmin(member)

	a.logger.Debug().
		Int64("chat_id", chatID).
		Int64("user_id", userID).
		Bool("is_admin", isAdmin).
		Str("status", member.Status).
		Msg("admin check completed")

	return domain.NewSuccessResult(map[string]any{
		"is_admin":  isAdmin,
		"chat_id":   chatID,
		"user_id":   userID,
		"tg_status": member.Status,
	}), nil
}
