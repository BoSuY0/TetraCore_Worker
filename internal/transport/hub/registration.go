package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
)

// registrationTimeout — максимальний час очікування відповіді реєстрації.
const registrationTimeout = 10 * time.Second

// register виконує повний цикл реєстрації на Hub:
// 1. Відправляє ClientRegistrationMessage.
// 2. Очікує ClientRegistrationResponse з таймаутом.
// 3. Зберігає session_id при успіху.
func (c *Client) register(ctx context.Context) error {
	regMsg := c.buildRegistrationMessage()

	data, err := json.Marshal(regMsg)
	if err != nil {
		return fmt.Errorf("marshal registration: %w", err)
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection is nil during registration")
	}

	// Встановлюємо дедлайн на читання відповіді.
	deadline := time.Now().Add(registrationTimeout)
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send registration: %w", err)
	}
	// Скидаємо дедлайн запису.
	_ = conn.SetWriteDeadline(time.Time{})

	c.logger.Debug().
		Str("client_id", c.config.ClientID).
		Int("task_types", len(c.config.SupportedTaskTypes)).
		Msg("registration message sent, waiting for response")

	// Очікуємо відповідь з таймаутом.
	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}

	_, respData, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read registration response: %w", err)
	}

	// Скидаємо дедлайн читання.
	_ = conn.SetReadDeadline(time.Time{})

	var resp ClientRegistrationResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return fmt.Errorf("parse registration response: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("hub rejected registration: %s", resp.Message)
	}

	c.setSessionID(resp.SessionID)
	c.logger.Info().
		Str("session_id", resp.SessionID).
		Str("client_id", resp.ClientID).
		Str("message", resp.Message).
		Msg("registration successful")

	return nil
}

// buildRegistrationMessage створює повідомлення реєстрації з можливостями воркера.
func (c *Client) buildRegistrationMessage() *ClientRegistrationMessage {
	return &ClientRegistrationMessage{
		BaseMessage: NewBaseMessage(MsgTypeClientRegistration, c.config.ClientID),
		ClientID:    c.config.ClientID,
		ClientType:  "worker",
		ClientName:  c.config.ClientName,
		Version:     "1.0.0",
		Capabilities: WorkerCapabilities{
			SupportedTaskTypes: c.config.SupportedTaskTypes,
			MaxConcurrentTasks: c.config.Concurrency,
			Version:            "1.0.0",
			OS:                 runtime.GOOS,
			Arch:               runtime.GOARCH,
		},
		AuthToken: c.config.AuthToken,
	}
}
