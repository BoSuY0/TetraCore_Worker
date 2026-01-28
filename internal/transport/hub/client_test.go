package hub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nopLogger повертає zerolog.Logger, що нікуди не пише (тихий логер для тестів).
func nopLogger() zerolog.Logger {
	return zerolog.Nop()
}

// testConfig повертає мінімальну HubConfig для тестів.
func testConfig() HubConfig {
	return HubConfig{
		URL:                "ws://test:8080/ws",
		AuthToken:          "test-secret-token",
		ClientID:           "worker-test-01",
		ClientName:         "TestWorker",
		PingInterval:       30 * time.Second,
		PingTimeout:        10 * time.Second,
		ReconnectDelay:     1 * time.Second,
		MaxReconnectDelay:  60 * time.Second,
		ConnectTimeout:     5 * time.Second,
		Concurrency:        4,
		SupportedTaskTypes: []string{"typeA", "typeB"},
	}
}

// emptyCallbacks повертає порожній Callbacks (без обробників).
func emptyCallbacks() Callbacks {
	return Callbacks{}
}

// newTestClient створює Client для тестів без реального підключення.
func newTestClient(cfg HubConfig, cb Callbacks) *Client {
	return NewClient(cfg, cb, nopLogger())
}

// ---------------------------------------------------------------------------
// 1. TestNewClient
// ---------------------------------------------------------------------------

func TestNewClient(t *testing.T) {
	cfg := testConfig()
	cb := emptyCallbacks()

	c := newTestClient(cfg, cb)

	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	// Початковий статус — Disconnected.
	if got := c.Status(); got != StatusDisconnected {
		t.Errorf("Status() = %v, want %v", got, StatusDisconnected)
	}

	// Конфігурація збережена коректно.
	if c.config.URL != cfg.URL {
		t.Errorf("config.URL = %q, want %q", c.config.URL, cfg.URL)
	}
	if c.config.ClientID != cfg.ClientID {
		t.Errorf("config.ClientID = %q, want %q", c.config.ClientID, cfg.ClientID)
	}
	if c.config.Concurrency != cfg.Concurrency {
		t.Errorf("config.Concurrency = %d, want %d", c.config.Concurrency, cfg.Concurrency)
	}

	// sendCh створено з буфером 256.
	if cap(c.sendCh) != 256 {
		t.Errorf("cap(sendCh) = %d, want 256", cap(c.sendCh))
	}

	// done-канал створено (не nil).
	if c.done == nil {
		t.Error("done channel is nil")
	}
}

// ---------------------------------------------------------------------------
// 2. TestClient_StatusTransitions
// ---------------------------------------------------------------------------

func TestClient_StatusTransitions(t *testing.T) {
	c := newTestClient(testConfig(), emptyCallbacks())

	transitions := []ConnectionStatus{
		StatusDisconnected,
		StatusConnecting,
		StatusConnected,
		StatusRegistered,
		StatusClosing,
		StatusDisconnected,
	}

	for i, want := range transitions {
		c.setStatus(want)
		got := c.Status()
		if got != want {
			t.Errorf("step %d: Status() = %v (%s), want %v (%s)",
				i, got, got.String(), want, want.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 3. TestClient_IsConnected
// ---------------------------------------------------------------------------

func TestClient_IsConnected(t *testing.T) {
	c := newTestClient(testConfig(), emptyCallbacks())

	tests := []struct {
		status ConnectionStatus
		want   bool
	}{
		{StatusDisconnected, false},
		{StatusConnecting, false},
		{StatusConnected, true},
		{StatusRegistered, true},
		{StatusClosing, false},
	}

	for _, tt := range tests {
		c.setStatus(tt.status)
		got := c.IsConnected()
		if got != tt.want {
			t.Errorf("IsConnected() with status %s = %v, want %v",
				tt.status.String(), got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. TestClient_Config
// ---------------------------------------------------------------------------

func TestClient_Config(t *testing.T) {
	cfg := testConfig()
	c := newTestClient(cfg, emptyCallbacks())

	got := c.Config()

	if got.URL != cfg.URL {
		t.Errorf("Config().URL = %q, want %q", got.URL, cfg.URL)
	}
	if got.AuthToken != cfg.AuthToken {
		t.Errorf("Config().AuthToken = %q, want %q", got.AuthToken, cfg.AuthToken)
	}
	if got.ClientID != cfg.ClientID {
		t.Errorf("Config().ClientID = %q, want %q", got.ClientID, cfg.ClientID)
	}
	if got.ClientName != cfg.ClientName {
		t.Errorf("Config().ClientName = %q, want %q", got.ClientName, cfg.ClientName)
	}
	if got.Concurrency != cfg.Concurrency {
		t.Errorf("Config().Concurrency = %d, want %d", got.Concurrency, cfg.Concurrency)
	}
	if got.PingInterval != cfg.PingInterval {
		t.Errorf("Config().PingInterval = %v, want %v", got.PingInterval, cfg.PingInterval)
	}
	if len(got.SupportedTaskTypes) != len(cfg.SupportedTaskTypes) {
		t.Errorf("Config().SupportedTaskTypes len = %d, want %d",
			len(got.SupportedTaskTypes), len(cfg.SupportedTaskTypes))
	}
}

// ---------------------------------------------------------------------------
// 5. TestClient_Done
// ---------------------------------------------------------------------------

func TestClient_Done(t *testing.T) {
	c := newTestClient(testConfig(), emptyCallbacks())

	doneCh := c.Done()
	if doneCh == nil {
		t.Fatal("Done() returned nil channel")
	}

	// Канал має бути відкритий (не заблокований на читанні).
	select {
	case <-doneCh:
		t.Fatal("Done() channel should not be closed initially")
	default:
		// Очікувана поведінка — канал відкритий, select потрапив у default.
	}

	// Закриваємо done-канал — Done() повинен сповістити.
	close(c.done)

	select {
	case <-doneCh:
		// Очікувана поведінка — канал закрився, сигнал отримано.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done() channel did not signal after close")
	}
}

// ---------------------------------------------------------------------------
// 6. TestBuildAuthHeaders_WithToken
// ---------------------------------------------------------------------------

func TestBuildAuthHeaders_WithToken(t *testing.T) {
	cfg := testConfig()
	c := newTestClient(cfg, emptyCallbacks())

	headers := c.buildAuthHeaders()

	requiredHeaders := []string{
		"Authorization",
		"X-Client-Id",
		"X-Timestamp",
		"X-Nonce",
		"X-Client-Type",
		"X-Client-Version",
		"X-Signature",
	}

	for _, h := range requiredHeaders {
		if v := headers.Get(h); v == "" {
			t.Errorf("header %q is missing or empty", h)
		}
	}

	// Перевірка значень.
	if got := headers.Get("Authorization"); got != "Bearer "+cfg.AuthToken {
		t.Errorf("Authorization = %q, want %q", got, "Bearer "+cfg.AuthToken)
	}
	if got := headers.Get("X-Client-Id"); got != cfg.ClientID {
		t.Errorf("X-Client-Id = %q, want %q", got, cfg.ClientID)
	}
	if got := headers.Get("X-Client-Type"); got != "worker" {
		t.Errorf("X-Client-Type = %q, want %q", got, "worker")
	}
	if got := headers.Get("X-Client-Version"); got != "1.0.0" {
		t.Errorf("X-Client-Version = %q, want %q", got, "1.0.0")
	}
}

// ---------------------------------------------------------------------------
// 7. TestBuildAuthHeaders_WithoutToken
// ---------------------------------------------------------------------------

func TestBuildAuthHeaders_WithoutToken(t *testing.T) {
	cfg := testConfig()
	cfg.AuthToken = "" // Порожній токен.
	c := newTestClient(cfg, emptyCallbacks())

	headers := c.buildAuthHeaders()

	// Заголовки мають бути порожніми.
	if len(headers) != 0 {
		t.Errorf("expected empty headers when AuthToken is empty, got %d headers", len(headers))
		for k, v := range headers {
			t.Logf("  %s: %v", k, v)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. TestBuildAuthHeaders_HMACSignature
// ---------------------------------------------------------------------------

func TestBuildAuthHeaders_HMACSignature(t *testing.T) {
	cfg := testConfig()
	c := newTestClient(cfg, emptyCallbacks())

	headers := c.buildAuthHeaders()

	// Витягуємо компоненти для перевірки підпису.
	clientID := headers.Get("X-Client-Id")
	ts := headers.Get("X-Timestamp")
	nonce := headers.Get("X-Nonce")
	clientType := headers.Get("X-Client-Type")
	version := headers.Get("X-Client-Version")
	signature := headers.Get("X-Signature")

	// Перезбираємо canonical string та обчислюємо HMAC.
	canonical := clientID + "|" + ts + "|" + nonce + "|" + clientType + "|" + version

	mac := hmac.New(sha256.New, []byte(cfg.AuthToken))
	mac.Write([]byte(canonical))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if signature != expectedSig {
		t.Errorf("HMAC signature mismatch\n  got:    %s\n  want:   %s\n  canonical: %s",
			signature, expectedSig, canonical)
	}
}

// ---------------------------------------------------------------------------
// 9. TestClient_Send_NotConnected
// ---------------------------------------------------------------------------

func TestClient_Send_NotConnected(t *testing.T) {
	c := newTestClient(testConfig(), emptyCallbacks())

	// Статус за замовчуванням — Disconnected.
	err := c.send("test-message")

	if err == nil {
		t.Fatal("send() should return error when disconnected")
	}

	if !errors.Is(err, domain.ErrHubDisconnected) {
		t.Errorf("send() error = %v, want %v", err, domain.ErrHubDisconnected)
	}
}

// ---------------------------------------------------------------------------
// 10. TestClient_SendWorkerStatus
// ---------------------------------------------------------------------------

func TestClient_SendWorkerStatus(t *testing.T) {
	cfg := testConfig()
	c := newTestClient(cfg, emptyCallbacks())

	// Встановлюємо статус Registered, щоб send() пропустив.
	c.setStatus(StatusRegistered)

	stats := domain.WorkerStats{
		WorkerID:       cfg.ClientID,
		State:          domain.WorkerStateIdle,
		ActiveTasks:    2,
		TotalProcessed: 100,
		TotalFailed:    3,
		UptimeSeconds:  3600,
		HubConnected:   true,
	}

	err := c.SendWorkerStatus(stats)
	if err != nil {
		t.Fatalf("SendWorkerStatus() returned error: %v", err)
	}

	// Перевіряємо, що повідомлення потрапило в sendCh.
	select {
	case data := <-c.sendCh:
		// Десеріалізуємо та перевіряємо структуру.
		var msg WorkerStatusMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("failed to unmarshal sent message: %v", err)
		}
		if msg.Type != MsgTypeWorkerStatus {
			t.Errorf("msg.Type = %q, want %q", msg.Type, MsgTypeWorkerStatus)
		}
		if msg.ClientID != cfg.ClientID {
			t.Errorf("msg.ClientID = %q, want %q", msg.ClientID, cfg.ClientID)
		}
		if msg.Status != string(domain.WorkerStateIdle) {
			t.Errorf("msg.Status = %q, want %q", msg.Status, domain.WorkerStateIdle)
		}
		if msg.ActiveTasks != 2 {
			t.Errorf("msg.ActiveTasks = %d, want 2", msg.ActiveTasks)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no message in sendCh after SendWorkerStatus()")
	}
}

// ---------------------------------------------------------------------------
// 11. TestDispatchMessage_Task
// ---------------------------------------------------------------------------

func TestDispatchMessage_Task(t *testing.T) {
	var received *TaskAssignmentMessage
	var mu sync.Mutex

	cb := Callbacks{
		OnTaskAssigned: func(msg *TaskAssignmentMessage) {
			mu.Lock()
			defer mu.Unlock()
			received = msg
		},
	}

	c := newTestClient(testConfig(), cb)

	taskMsg := TaskAssignmentMessage{
		BaseMessage: BaseMessage{
			Type:      MsgTypeTask,
			Timestamp: time.Now().UTC(),
			MessageID: "msg-001",
			SenderID:  "hub",
		},
		TaskID:   "task-123",
		TaskType: "typeA",
		Priority: "high",
		Payload:  map[string]any{"key": "value"},
	}

	data, err := json.Marshal(taskMsg)
	if err != nil {
		t.Fatalf("failed to marshal task message: %v", err)
	}

	c.dispatchMessage(data)

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("OnTaskAssigned callback was not called")
	}
	if received.TaskID != "task-123" {
		t.Errorf("received.TaskID = %q, want %q", received.TaskID, "task-123")
	}
	if received.TaskType != "typeA" {
		t.Errorf("received.TaskType = %q, want %q", received.TaskType, "typeA")
	}
}

// ---------------------------------------------------------------------------
// 12. TestDispatchMessage_TaskCancel
// ---------------------------------------------------------------------------

func TestDispatchMessage_TaskCancel(t *testing.T) {
	var received *TaskCancelMessage
	var mu sync.Mutex

	cb := Callbacks{
		OnTaskCancelled: func(msg *TaskCancelMessage) {
			mu.Lock()
			defer mu.Unlock()
			received = msg
		},
	}

	c := newTestClient(testConfig(), cb)

	cancelMsg := TaskCancelMessage{
		BaseMessage: BaseMessage{
			Type:      MsgTypeTaskCancel,
			Timestamp: time.Now().UTC(),
			MessageID: "msg-002",
			SenderID:  "hub",
		},
		TaskID: "task-456",
		Reason: "user requested cancellation",
	}

	data, err := json.Marshal(cancelMsg)
	if err != nil {
		t.Fatalf("failed to marshal cancel message: %v", err)
	}

	c.dispatchMessage(data)

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("OnTaskCancelled callback was not called")
	}
	if received.TaskID != "task-456" {
		t.Errorf("received.TaskID = %q, want %q", received.TaskID, "task-456")
	}
	if received.Reason != "user requested cancellation" {
		t.Errorf("received.Reason = %q, want %q", received.Reason, "user requested cancellation")
	}
}

// ---------------------------------------------------------------------------
// 13. TestDispatchMessage_Error
// ---------------------------------------------------------------------------

func TestDispatchMessage_Error(t *testing.T) {
	var received *ErrorMessage
	var mu sync.Mutex

	cb := Callbacks{
		OnError: func(msg *ErrorMessage) {
			mu.Lock()
			defer mu.Unlock()
			received = msg
		},
	}

	c := newTestClient(testConfig(), cb)

	errMsg := ErrorMessage{
		BaseMessage: BaseMessage{
			Type:      MsgTypeError,
			Timestamp: time.Now().UTC(),
			MessageID: "msg-003",
			SenderID:  "hub",
		},
		Code:    "RATE_LIMIT",
		Message: "too many requests",
	}

	data, err := json.Marshal(errMsg)
	if err != nil {
		t.Fatalf("failed to marshal error message: %v", err)
	}

	c.dispatchMessage(data)

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("OnError callback was not called")
	}
	if received.Code != "RATE_LIMIT" {
		t.Errorf("received.Code = %q, want %q", received.Code, "RATE_LIMIT")
	}
	if received.Message != "too many requests" {
		t.Errorf("received.Message = %q, want %q", received.Message, "too many requests")
	}
}

// ---------------------------------------------------------------------------
// 14. TestDispatchMessage_Pong
// ---------------------------------------------------------------------------

func TestDispatchMessage_Pong(t *testing.T) {
	c := newTestClient(testConfig(), emptyCallbacks())

	// Обнулюємо LastPongAt.
	c.stats.LastPongAt.Store(0)

	before := time.Now().Unix()

	pongMsg := BaseMessage{
		Type:      MsgTypePong,
		Timestamp: time.Now().UTC(),
		MessageID: "msg-004",
		SenderID:  "hub",
	}

	data, err := json.Marshal(pongMsg)
	if err != nil {
		t.Fatalf("failed to marshal pong message: %v", err)
	}

	c.dispatchMessage(data)

	after := time.Now().Unix()
	lastPong := c.stats.LastPongAt.Load()

	if lastPong < before || lastPong > after {
		t.Errorf("LastPongAt = %d, expected in range [%d, %d]", lastPong, before, after)
	}
}

// ---------------------------------------------------------------------------
// 15. TestDispatchMessage_Unknown
// ---------------------------------------------------------------------------

func TestDispatchMessage_Unknown(t *testing.T) {
	c := newTestClient(testConfig(), emptyCallbacks())

	unknownMsg := BaseMessage{
		Type:      "some_unknown_type",
		Timestamp: time.Now().UTC(),
		MessageID: "msg-005",
		SenderID:  "hub",
	}

	data, err := json.Marshal(unknownMsg)
	if err != nil {
		t.Fatalf("failed to marshal unknown message: %v", err)
	}

	// Не повинен панікувати.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dispatchMessage panicked on unknown type: %v", r)
		}
	}()

	c.dispatchMessage(data)

	// Додаткова перевірка: невалідний JSON теж не повинен панікувати.
	c.dispatchMessage([]byte(`{invalid-json`))
}

// ---------------------------------------------------------------------------
// Додатково: TestConnectionStatus_String — для повноти покриття.
// ---------------------------------------------------------------------------

func TestConnectionStatus_String(t *testing.T) {
	tests := []struct {
		status ConnectionStatus
		want   string
	}{
		{StatusDisconnected, "disconnected"},
		{StatusConnecting, "connecting"},
		{StatusConnected, "connected"},
		{StatusRegistered, "registered"},
		{StatusClosing, "closing"},
		{ConnectionStatus(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("ConnectionStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Додатково: TestDefaultHubConfig — перевірка значень за замовчуванням.
// ---------------------------------------------------------------------------

func TestDefaultHubConfig(t *testing.T) {
	cfg := DefaultHubConfig()

	if !strings.HasPrefix(cfg.URL, "ws://") {
		t.Errorf("DefaultHubConfig().URL = %q, want ws:// prefix", cfg.URL)
	}
	if cfg.PingInterval != 30*time.Second {
		t.Errorf("PingInterval = %v, want 30s", cfg.PingInterval)
	}
	if cfg.PingTimeout != 10*time.Second {
		t.Errorf("PingTimeout = %v, want 10s", cfg.PingTimeout)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("Concurrency = %d, want 5", cfg.Concurrency)
	}
}
