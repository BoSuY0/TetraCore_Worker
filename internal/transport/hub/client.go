package hub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
)

// ---------------------------------------------------------------------------
// ConnectionStatus — стани WebSocket-з'єднання.
// ---------------------------------------------------------------------------

// ConnectionStatus відображає поточний стан з'єднання з Hub.
type ConnectionStatus int32

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusRegistered
	StatusClosing
)

// String повертає текстове представлення стану.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "disconnected"
	case StatusConnecting:
		return "connecting"
	case StatusConnected:
		return "connected"
	case StatusRegistered:
		return "registered"
	case StatusClosing:
		return "closing"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// HubConfig — конфігурація підключення до Hub.
// ---------------------------------------------------------------------------

// HubConfig містить всі параметри для WebSocket-клієнта Hub.
type HubConfig struct {
	URL                string        `json:"url"                mapstructure:"url"`
	AuthToken          string        `json:"auth_token"         mapstructure:"auth_token"`
	ClientID           string        `json:"client_id"          mapstructure:"client_id"`
	ClientName         string        `json:"client_name"        mapstructure:"client_name"`
	PingInterval       time.Duration `json:"ping_interval"      mapstructure:"ping_interval"`
	PingTimeout        time.Duration `json:"ping_timeout"       mapstructure:"ping_timeout"`
	ReconnectDelay     time.Duration `json:"reconnect_delay"    mapstructure:"reconnect_delay"`
	MaxReconnectDelay  time.Duration `json:"max_reconnect_delay" mapstructure:"max_reconnect_delay"`
	ConnectTimeout     time.Duration `json:"connect_timeout"    mapstructure:"connect_timeout"`
	Concurrency        int           `json:"concurrency"        mapstructure:"concurrency"`
	SupportedTaskTypes []string      `json:"supported_task_types,omitempty"` // динамічний список з Registry
}

// DefaultHubConfig повертає конфігурацію за замовчуванням.
func DefaultHubConfig() HubConfig {
	return HubConfig{
		URL:               "ws://localhost:8080/ws",
		PingInterval:      30 * time.Second,
		PingTimeout:       10 * time.Second,
		ReconnectDelay:    5 * time.Second,
		MaxReconnectDelay: 300 * time.Second,
		ConnectTimeout:    10 * time.Second,
		Concurrency:       5,
	}
}

// ---------------------------------------------------------------------------
// Callbacks — функції зворотного виклику для обробки вхідних повідомлень.
// ---------------------------------------------------------------------------

// Callbacks визначає обробники подій від Hub.
type Callbacks struct {
	OnTaskAssigned  func(msg *TaskAssignmentMessage)
	OnTaskCancelled func(msg *TaskCancelMessage)
	OnError         func(msg *ErrorMessage)
}

// ---------------------------------------------------------------------------
// ClientStats — статистика WebSocket-клієнта.
// ---------------------------------------------------------------------------

// ClientStats зберігає лічильники активності з'єднання.
type ClientStats struct {
	MessagesSent     atomic.Int64
	MessagesReceived atomic.Int64
	LastPongAt       atomic.Int64 // Unix timestamp
	ConnectedAt      time.Time
	ReconnectCount   atomic.Int64
}

// ---------------------------------------------------------------------------
// Client — WebSocket-клієнт для з'єднання з Hub.
// ---------------------------------------------------------------------------

// Client реалізує WebSocket-з'єднання з Hub: підключення, реєстрація,
// прийом/надсилання повідомлень, heartbeat, реконнект.
type Client struct {
	config    HubConfig
	conn      *websocket.Conn
	sessionID string
	status    atomic.Int32 // ConnectionStatus

	sendCh chan []byte
	done   chan struct{}

	callbacks Callbacks
	stats     ClientStats
	logger    zerolog.Logger

	mu sync.RWMutex // захищає conn та sessionID
}

// NewClient створює новий WebSocket-клієнт до Hub.
func NewClient(cfg HubConfig, callbacks Callbacks, logger zerolog.Logger) *Client {
	c := &Client{
		config:    cfg,
		sendCh:    make(chan []byte, 256),
		done:      make(chan struct{}),
		callbacks: callbacks,
		logger:    logger.With().Str("component", "hub_client").Logger(),
	}
	c.setStatus(StatusDisconnected)
	return c
}

// ---------------------------------------------------------------------------
// Public API.
// ---------------------------------------------------------------------------

// Connect встановлює WebSocket-з'єднання, виконує HMAC-автентифікацію та
// реєстрацію клієнта на Hub. Запускає горутини read/write.
func (c *Client) Connect(ctx context.Context) error {
	c.setStatus(StatusConnecting)

	c.logger.Info().
		Str("url", c.config.URL).
		Str("client_id", c.config.ClientID).
		Msg("connecting to hub")

	// Побудова HMAC-заголовків автентифікації.
	headers := c.buildAuthHeaders()

	// Dial з таймаутом.
	dialer := websocket.Dialer{
		HandshakeTimeout: c.config.ConnectTimeout,
		Subprotocols:     []string{"bearer.hmac.v1"},
	}

	conn, resp, err := dialer.DialContext(ctx, c.config.URL, headers)
	if err != nil {
		c.setStatus(StatusDisconnected)
		if resp != nil {
			c.logger.Error().
				Int("http_status", resp.StatusCode).
				Err(err).
				Msg("websocket dial failed")
		}
		return fmt.Errorf("dial hub: %w", err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Встановлюємо WS-level ping handler для оновлення LastPongAt.
	// Hub надсилає WebSocket ping frames — gorilla автоматично відповідає pong,
	// але ми також оновлюємо час останнього пінгу для моніторингу здоров'я.
	conn.SetPingHandler(func(appData string) error {
		c.stats.LastPongAt.Store(time.Now().Unix())
		c.logger.Debug().Msg("ws-level ping received from hub")
		// Відповідаємо pong через gorilla default handler.
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	c.setStatus(StatusConnected)
	c.stats.ConnectedAt = time.Now()

	c.logger.Info().Msg("websocket connected, starting registration")

	// Реєстрація клієнта.
	if err := c.register(ctx); err != nil {
		c.closeConn()
		c.setStatus(StatusDisconnected)
		return fmt.Errorf("%w: %v", domain.ErrHubRegistration, err)
	}

	c.setStatus(StatusRegistered)
	c.logger.Info().
		Str("session_id", c.getSessionID()).
		Msg("registered on hub")

	// Оновлюємо done-канал для нового з'єднання.
	c.mu.Lock()
	c.done = make(chan struct{})
	c.mu.Unlock()

	// Запуск горутин.
	go c.readLoop()
	go c.writeLoop()

	return nil
}

// Disconnect виконує graceful закриття з'єднання.
func (c *Client) Disconnect() {
	if c.Status() == StatusDisconnected || c.Status() == StatusClosing {
		return
	}

	c.setStatus(StatusClosing)
	c.logger.Info().Msg("disconnecting from hub")

	c.closeConn()
	c.setStatus(StatusDisconnected)

	c.logger.Info().Msg("disconnected from hub")
}

// SendTaskResult надсилає результат виконання завдання до Hub.
func (c *Client) SendTaskResult(result domain.TaskResult) error {
	var errInfo *TaskErrorInfo
	if result.Error != nil {
		errInfo = &TaskErrorInfo{
			Type:    result.Error.Type,
			Message: result.Error.Message,
		}
	}

	msg := NewTaskResultMsg(
		c.config.ClientID,
		result.TaskID,
		string(result.Status),
		result.Result,
		errInfo,
		result.ExecutionTime,
	)

	return c.send(msg)
}

// SendWorkerStatus надсилає поточний стан воркера до Hub.
func (c *Client) SendWorkerStatus(stats domain.WorkerStats) error {
	metrics := map[string]any{
		"total_processed": stats.TotalProcessed,
		"total_failed":    stats.TotalFailed,
		"uptime_seconds":  stats.UptimeSeconds,
		"hub_connected":   stats.HubConnected,
	}
	if stats.CPUUsage != nil {
		metrics["cpu_usage"] = *stats.CPUUsage
	}
	if stats.MemoryUsage != nil {
		metrics["memory_usage"] = *stats.MemoryUsage
	}

	msg := NewWorkerStatusMsg(
		c.config.ClientID,
		c.config.ClientID,
		string(stats.State),
		stats.ActiveTasks,
		float64(stats.ActiveTasks)/float64(max(c.config.Concurrency, 1)),
		metrics,
	)

	return c.send(msg)
}

// SendProgress надсилає повідомлення про прогрес виконання завдання.
func (c *Client) SendProgress(taskID string, progress float64, message string) error {
	msg := NewTaskProgressMsg(c.config.ClientID, taskID, progress, message, nil)
	return c.send(msg)
}

// IsConnected повертає true, якщо з'єднання активне та зареєстроване.
func (c *Client) IsConnected() bool {
	st := ConnectionStatus(c.status.Load())
	return st == StatusConnected || st == StatusRegistered
}

// Status повертає поточний стан з'єднання.
func (c *Client) Status() ConnectionStatus {
	return ConnectionStatus(c.status.Load())
}

// Stats повертає статистику клієнта.
func (c *Client) Stats() *ClientStats {
	return &c.stats
}

// Done повертає канал, що закривається при розриві з'єднання.
func (c *Client) Done() <-chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.done
}

// SessionID повертає поточний ідентифікатор сесії від Hub.
func (c *Client) SessionID() string {
	return c.getSessionID()
}

// Config повертає поточну конфігурацію Hub-клієнта.
func (c *Client) Config() HubConfig {
	return c.config
}

// ---------------------------------------------------------------------------
// Internal: read/write loops.
// ---------------------------------------------------------------------------

// readLoop читає повідомлення з WebSocket і передає їх обробникам.
func (c *Client) readLoop() {
	defer func() {
		c.mu.RLock()
		done := c.done
		c.mu.RUnlock()
		select {
		case <-done:
			// Вже закрито.
		default:
			close(done)
		}
	}()

	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			if c.Status() != StatusClosing {
				c.logger.Warn().Err(err).Msg("websocket read error")
			}
			return
		}

		c.stats.MessagesReceived.Add(1)
		c.dispatchMessage(data)
	}
}

// writeLoop записує повідомлення з каналу sendCh у WebSocket.
func (c *Client) writeLoop() {
	for {
		select {
		case data, ok := <-c.sendCh:
			if !ok {
				return
			}

			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.logger.Error().Err(err).Msg("websocket write error")
				return
			}

			c.stats.MessagesSent.Add(1)

		case <-c.Done():
			// Дренуємо залишкові повідомлення з логуванням.
			dropped := 0
			for {
				select {
				case <-c.sendCh:
					dropped++
				default:
					if dropped > 0 {
						c.logger.Warn().Int("dropped", dropped).Msg("messages dropped during disconnect")
					}
					return
				}
			}
		}
	}
}

// dispatchMessage розбирає тип повідомлення і викликає відповідний callback.
func (c *Client) dispatchMessage(data []byte) {
	// Спочатку витягуємо тип повідомлення.
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		c.logger.Error().Err(err).Str("raw", string(data)).Msg("failed to parse base message")
		return
	}

	c.logger.Debug().
		Str("type", base.Type).
		Str("message_id", base.MessageID).
		Msg("received message")

	switch base.Type {
	case MsgTypeTask:
		var msg TaskAssignmentMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error().Err(err).Msg("failed to parse task assignment")
			return
		}
		if c.callbacks.OnTaskAssigned != nil {
			c.callbacks.OnTaskAssigned(&msg)
		}

	case MsgTypeTaskCancel:
		var msg TaskCancelMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error().Err(err).Msg("failed to parse task cancel")
			return
		}
		if c.callbacks.OnTaskCancelled != nil {
			c.callbacks.OnTaskCancelled(&msg)
		}

	case MsgTypeHeartbeat:
		var msg HeartbeatMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error().Err(err).Msg("failed to parse heartbeat")
			return
		}
		c.handleHeartbeat(&msg)

	case MsgTypeError:
		var msg ErrorMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error().Err(err).Msg("failed to parse error message")
			return
		}
		c.logger.Warn().
			Str("code", msg.Code).
			Str("message", msg.Message).
			Msg("hub error received")
		if c.callbacks.OnError != nil {
			c.callbacks.OnError(&msg)
		}

	case MsgTypePong:
		// Hub відповів pong на наш heartbeat — оновлюємо LastPongAt.
		c.stats.LastPongAt.Store(time.Now().Unix())
		c.logger.Debug().Msg("pong received from hub")

	default:
		c.logger.Debug().
			Str("type", base.Type).
			Msg("unhandled message type")
	}
}

// ---------------------------------------------------------------------------
// Internal: send helper.
// ---------------------------------------------------------------------------

// send серіалізує повідомлення в JSON і додає до черги відправки.
func (c *Client) send(msg any) error {
	if !c.IsConnected() {
		return domain.ErrHubDisconnected
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	select {
	case c.sendCh <- data:
		return nil
	default:
		return fmt.Errorf("send channel full, message dropped")
	}
}

// ---------------------------------------------------------------------------
// Internal: auth & connection helpers.
// ---------------------------------------------------------------------------

// buildAuthHeaders будує HTTP-заголовки HMAC-автентифікації для WebSocket handshake.
func (c *Client) buildAuthHeaders() http.Header {
	headers := http.Header{}

	if c.config.AuthToken == "" {
		return headers
	}

	clientType := "worker"
	version := "1.0.0"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := uuid.New().String()

	// canonical string: clientID|timestamp|nonce|clientType|version
	canonical := c.config.ClientID + "|" + ts + "|" + nonce + "|" + clientType + "|" + version

	mac := hmac.New(sha256.New, []byte(c.config.AuthToken))
	mac.Write([]byte(canonical))
	signature := hex.EncodeToString(mac.Sum(nil))

	headers.Set("Authorization", "Bearer "+c.config.AuthToken)
	headers.Set("X-Client-Id", c.config.ClientID)
	headers.Set("X-Timestamp", ts)
	headers.Set("X-Nonce", nonce)
	headers.Set("X-Client-Type", clientType)
	headers.Set("X-Client-Version", version)
	headers.Set("X-Signature", signature)

	return headers
}

// closeConn закриває WebSocket-з'єднання та очищає посилання.
func (c *Client) closeConn() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		// Надсилаємо Close frame для graceful shutdown.
		_ = conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		_ = conn.Close()
	}
}

// setStatus атомарно змінює стан з'єднання.
func (c *Client) setStatus(s ConnectionStatus) {
	c.status.Store(int32(s))
}

// getSessionID безпечно повертає session_id.
func (c *Client) getSessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// setSessionID безпечно встановлює session_id.
func (c *Client) setSessionID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = id
}
