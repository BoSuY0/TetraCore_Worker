// Package telegram надає HTTP-клієнт для взаємодії з Telegram Bot API.
// Використовується action-ами воркера для перевірки прав користувачів у чатах.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
)

// ---------------------------------------------------------------------------
// Configuration.
// ---------------------------------------------------------------------------

// Config — конфігурація Telegram HTTP-клієнта.
type Config struct {
	BotToken string `json:"bot_token" mapstructure:"bot_token"`
	APIURL   string `json:"api_url"   mapstructure:"api_url"`
	Timeout  int    `json:"timeout"   mapstructure:"timeout"` // секунди
}

// DefaultConfig повертає конфігурацію за замовчуванням.
func DefaultConfig() Config {
	return Config{
		APIURL:  "https://api.telegram.org",
		Timeout: 10,
	}
}

// ---------------------------------------------------------------------------
// Option — функціональні опції для клієнта.
// ---------------------------------------------------------------------------

// Option визначає функціональну опцію для Client.
type Option func(*Client)

// WithHTTPClient замінює HTTP-клієнт за замовчуванням.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithLogger встановлює логер.
func WithLogger(logger zerolog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithAPIURL перевизначає базовий URL Telegram API.
func WithAPIURL(url string) Option {
	return func(c *Client) {
		c.apiURL = url
	}
}

// ---------------------------------------------------------------------------
// Telegram API response types.
// ---------------------------------------------------------------------------

// TelegramUser — інформація про користувача Telegram.
type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// ChatMember — інформація про учасника чату.
type ChatMember struct {
	Status string        `json:"status"`
	User   *TelegramUser `json:"user,omitempty"`
}

// IsAdmin повертає true, якщо учасник є адміністратором або засновником.
func IsAdmin(member *ChatMember) bool {
	if member == nil {
		return false
	}
	return member.Status == "creator" || member.Status == "administrator"
}

// IsMember повертає true, якщо учасник є частиною чату
// (будь-який статус, крім "left" та "kicked").
func IsMember(member *ChatMember) bool {
	if member == nil {
		return false
	}
	return member.Status != "left" && member.Status != "kicked"
}

// apiResponse — загальна обгортка відповіді Telegram Bot API.
type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

// ---------------------------------------------------------------------------
// Client — HTTP-клієнт для Telegram Bot API.
// ---------------------------------------------------------------------------

// Client надає методи для взаємодії з Telegram Bot API через HTTP.
// Використовує стандартний net/http, без зовнішніх SDK.
type Client struct {
	botToken   string
	apiURL     string
	httpClient *http.Client
	logger     zerolog.Logger
}

// NewClient створює новий Telegram HTTP-клієнт.
// Приймає Config та функціональні опції.
func NewClient(cfg Config, opts ...Option) *Client {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "https://api.telegram.org"
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	c := &Client{
		botToken: cfg.BotToken,
		apiURL:   apiURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: zerolog.Nop(),
	}

	for _, opt := range opts {
		opt(c)
	}

	c.logger = c.logger.With().Str("component", "telegram_client").Logger()
	return c
}

// ---------------------------------------------------------------------------
// Public API.
// ---------------------------------------------------------------------------

// GetChatMember запитує інформацію про учасника чату через Telegram Bot API.
//
// HTTP GET https://api.telegram.org/bot{token}/getChatMember?chat_id={chatID}&user_id={userID}
//
// Повертає ChatMember або помилку.
func (c *Client) GetChatMember(ctx context.Context, chatID int64, userID int64) (*ChatMember, error) {
	url := fmt.Sprintf(
		"%s/bot%s/getChatMember?chat_id=%s&user_id=%s",
		c.apiURL,
		c.botToken,
		strconv.FormatInt(chatID, 10),
		strconv.FormatInt(userID, 10),
	)

	c.logger.Debug().
		Int64("chat_id", chatID).
		Int64("user_id", userID).
		Msg("requesting getChatMember")

	result, err := c.doRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	var member ChatMember
	if err := json.Unmarshal(result, &member); err != nil {
		return nil, fmt.Errorf("parse getChatMember result: %w", err)
	}

	c.logger.Debug().
		Int64("chat_id", chatID).
		Int64("user_id", userID).
		Str("status", member.Status).
		Msg("getChatMember success")

	return &member, nil
}

// IsUserAdmin перевіряє, чи є користувач адміністратором у чаті.
// Комбінує GetChatMember + IsAdmin в один виклик.
func (c *Client) IsUserAdmin(ctx context.Context, chatID int64, userID int64) (bool, error) {
	member, err := c.GetChatMember(ctx, chatID, userID)
	if err != nil {
		return false, err
	}
	return IsAdmin(member), nil
}

// ---------------------------------------------------------------------------
// Internal HTTP helper.
// ---------------------------------------------------------------------------

// doRequest виконує HTTP-запит та обробляє відповідь Telegram API.
// Повертає json.RawMessage з полем "result" або помилку.
func (c *Client) doRequest(ctx context.Context, method, url string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrTelegramAPI, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// Перевірка HTTP-статусу.
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn().
			Int("status_code", resp.StatusCode).
			Str("body", truncateBody(body, 512)).
			Msg("telegram API non-200 response")
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse telegram response: %w", err)
	}

	if !apiResp.OK {
		c.logger.Warn().
			Int("error_code", apiResp.ErrorCode).
			Str("description", apiResp.Description).
			Msg("telegram API error")
		return nil, fmt.Errorf("%w: [%d] %s", domain.ErrTelegramAPI, apiResp.ErrorCode, apiResp.Description)
	}

	return apiResp.Result, nil
}

// truncateBody обрізає тіло відповіді для логування.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
