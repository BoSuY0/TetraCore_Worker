package mysql

import (
	"time"
)

// ---------------------------------------------------------------------------
// Enum-типи (відповідають SQLAlchemy-моделям Python Bot)
// ---------------------------------------------------------------------------

// ChatType — тип Telegram-чату.
type ChatType string

const (
	ChatTypePrivate    ChatType = "private"
	ChatTypeGroup      ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
	ChatTypeChannel    ChatType = "channel"
	ChatTypeUnknown    ChatType = "unknown"
)

// SubscriptionType — тип підписки користувача.
type SubscriptionType string

const (
	SubscriptionFree    SubscriptionType = "free"
	SubscriptionPro     SubscriptionType = "pro"
	SubscriptionProPlus SubscriptionType = "pro_plus"
	SubscriptionTetra   SubscriptionType = "tetra"
	SubscriptionAdmin   SubscriptionType = "admin"
	SubscriptionTester  SubscriptionType = "tester"
)

// ---------------------------------------------------------------------------
// GORM-моделі (відповідають Alembic-міграціям Python Bot)
// ---------------------------------------------------------------------------

// Chat — модель таблиці "chats".
type Chat struct {
	ID          int        `gorm:"primaryKey;autoIncrement"`
	ChatID      int64      `gorm:"column:chat_id;uniqueIndex;not null"`
	ChatType    string     `gorm:"column:chat_type;type:varchar(32);not null"`
	Title       *string    `gorm:"column:title;type:varchar(255)"`
	MemberCount *int       `gorm:"column:member_count"`
	OwnerID     *int64     `gorm:"column:owner_user_id"`
	Language    string     `gorm:"column:language;type:varchar(10);not null"`
	IsBotAdmin  bool       `gorm:"column:is_bot_admin;not null;default:false"`
	IsForum     bool       `gorm:"column:is_forum;not null;default:false"`
	IsActive    bool       `gorm:"column:is_active;not null;default:false"`
	CreatedAt   *time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null"`
}

// TableName повертає ім'я таблиці для GORM.
func (Chat) TableName() string { return "chats" }

// User — модель таблиці "users".
type User struct {
	ID                int        `gorm:"primaryKey;autoIncrement"`
	UserID            int64      `gorm:"column:user_id;uniqueIndex;not null"`
	Username          *string    `gorm:"column:username;type:varchar(255)"`
	FirstName         *string    `gorm:"column:first_name;type:varchar(255)"`
	Language          *string    `gorm:"column:language;type:varchar(10)"`
	SubscriptionType  string     `gorm:"column:subscription_type;type:varchar(32);not null;default:'free'"`
	SubscriptionStart *time.Time `gorm:"column:subscription_start"`
	SubscriptionEnd   *time.Time `gorm:"column:subscription_end_date"`
	Experience        int        `gorm:"column:experience;not null;default:0"`
	Level             int        `gorm:"column:level;not null;default:1"`
	CreatedAt         *time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`
}

// TableName повертає ім'я таблиці для GORM.
func (User) TableName() string { return "users" }

// ChatModule — модель таблиці "chat_modules".
type ChatModule struct {
	ID                  int        `gorm:"primaryKey;autoIncrement"`
	ChatID              int64      `gorm:"column:chat_id;not null"`
	ModuleName          string     `gorm:"column:module_name;type:varchar(50);not null"`
	IsActive            bool       `gorm:"column:is_active;not null;default:false"`
	ActivatedByUserID   *int64     `gorm:"column:activated_by_user_id"`
	DeactivatedByUserID *int64     `gorm:"column:deactivated_by_user_id"`
	ActivatedAt         *time.Time `gorm:"column:activated_at"`
	DeactivatedAt       *time.Time `gorm:"column:deactivated_at"`
	Settings            *string    `gorm:"column:settings;type:json"`
}

// TableName повертає ім'я таблиці для GORM.
func (ChatModule) TableName() string { return "chat_modules" }

// ---------------------------------------------------------------------------
// Допоміжні функції перетворення
// ---------------------------------------------------------------------------

// ChatToMap конвертує Chat у map[string]any для відповіді API.
// Включає лише безпечні поля (без внутрішніх ID).
// Поле "subscription" отримується з ownerSubscription — типу підписки
// власника чату (має бути отримано через JOIN з таблицею users).
func ChatToMap(chat *Chat, ownerSubscription string) map[string]any {
	if chat == nil {
		return nil
	}

	result := map[string]any{
		"chat_id":      chat.ChatID,
		"chat_type":    chat.ChatType,
		"language":     chat.Language,
		"is_forum":     chat.IsForum,
		"is_active":    chat.IsActive,
		"is_bot_admin": chat.IsBotAdmin,
		"updated_at":   chat.UpdatedAt.Format(time.RFC3339),
	}

	// Nullable-поля додаємо тільки якщо вони не nil.
	if chat.Title != nil {
		result["title"] = *chat.Title
	} else {
		result["title"] = nil
	}

	if chat.CreatedAt != nil {
		result["created_at"] = chat.CreatedAt.Format(time.RFC3339)
	} else {
		result["created_at"] = nil
	}

	// Підписка визначається через власника чату.
	if ownerSubscription != "" {
		result["subscription"] = ownerSubscription
	} else {
		result["subscription"] = string(SubscriptionFree)
	}

	return result
}

// UserToMap конвертує User у map[string]any для відповіді API.
func UserToMap(user *User) map[string]any {
	if user == nil {
		return nil
	}

	result := map[string]any{
		"user_id":           user.UserID,
		"subscription_type": user.SubscriptionType,
		"experience":        user.Experience,
		"level":             user.Level,
		"updated_at":        user.UpdatedAt.Format(time.RFC3339),
	}

	if user.Username != nil {
		result["username"] = *user.Username
	} else {
		result["username"] = nil
	}

	if user.FirstName != nil {
		result["first_name"] = *user.FirstName
	} else {
		result["first_name"] = nil
	}

	if user.Language != nil {
		result["language"] = *user.Language
	} else {
		result["language"] = nil
	}

	if user.SubscriptionStart != nil {
		result["subscription_start"] = user.SubscriptionStart.Format(time.RFC3339)
	} else {
		result["subscription_start"] = nil
	}

	if user.SubscriptionEnd != nil {
		result["subscription_end_date"] = user.SubscriptionEnd.Format(time.RFC3339)
	} else {
		result["subscription_end_date"] = nil
	}

	if user.CreatedAt != nil {
		result["created_at"] = user.CreatedAt.Format(time.RFC3339)
	} else {
		result["created_at"] = nil
	}

	return result
}

// ChatModuleToMap конвертує ChatModule у map[string]any для відповіді API.
func ChatModuleToMap(module *ChatModule) map[string]any {
	if module == nil {
		return nil
	}

	result := map[string]any{
		"chat_id":     module.ChatID,
		"module_name": module.ModuleName,
		"is_active":   module.IsActive,
	}

	if module.Settings != nil {
		result["settings"] = *module.Settings
	} else {
		result["settings"] = nil
	}

	if module.ActivatedByUserID != nil {
		result["activated_by_user_id"] = *module.ActivatedByUserID
	}

	if module.DeactivatedByUserID != nil {
		result["deactivated_by_user_id"] = *module.DeactivatedByUserID
	}

	if module.ActivatedAt != nil {
		result["activated_at"] = module.ActivatedAt.Format(time.RFC3339)
	} else {
		result["activated_at"] = nil
	}

	if module.DeactivatedAt != nil {
		result["deactivated_at"] = module.DeactivatedAt.Format(time.RFC3339)
	} else {
		result["deactivated_at"] = nil
	}

	return result
}
