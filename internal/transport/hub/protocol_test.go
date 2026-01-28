package hub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. TestNewBaseMessage
// ---------------------------------------------------------------------------

func TestNewBaseMessage(t *testing.T) {
	before := time.Now().UTC()
	msg := NewBaseMessage(MsgTypeTaskResult, "worker-1")
	after := time.Now().UTC()

	// Type повинен збігатися з переданим msgType.
	if msg.Type != MsgTypeTaskResult {
		t.Errorf("Type: got %q, want %q", msg.Type, MsgTypeTaskResult)
	}

	// MessageType повинен дублювати Type (string-копія).
	if msg.MessageType != msg.Type {
		t.Errorf("MessageType (%q) != Type (%q)", msg.MessageType, msg.Type)
	}

	// Timestamp — не нульовий, UTC, в межах [before, after].
	if msg.Timestamp.IsZero() {
		t.Fatal("Timestamp is zero")
	}
	if msg.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp location: got %v, want UTC", msg.Timestamp.Location())
	}
	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Errorf("Timestamp %v not in [%v, %v]", msg.Timestamp, before, after)
	}

	// MessageID — не порожній, має формат UUID (36 символів із дефісами).
	if msg.MessageID == "" {
		t.Fatal("MessageID is empty")
	}
	if len(msg.MessageID) != 36 {
		t.Errorf("MessageID length: got %d, want 36 (UUID v4)", len(msg.MessageID))
	}

	// SenderID — збігається з переданим.
	if msg.SenderID != "worker-1" {
		t.Errorf("SenderID: got %q, want %q", msg.SenderID, "worker-1")
	}
}

// ---------------------------------------------------------------------------
// 2. TestBaseMessage_JSONSerialization
// ---------------------------------------------------------------------------

func TestBaseMessage_JSONSerialization(t *testing.T) {
	msg := NewBaseMessage(MsgTypeClientRegistration, "w-42")

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	raw := string(data)

	// JSON повинен містити обидва ключі "type" та "message_type".
	if !strings.Contains(raw, `"type"`) {
		t.Error(`JSON does not contain "type" field`)
	}
	if !strings.Contains(raw, `"message_type"`) {
		t.Error(`JSON does not contain "message_type" field`)
	}

	// Десеріалізація назад.
	var decoded BaseMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Type != msg.Type {
		t.Errorf("Round-trip Type: got %q, want %q", decoded.Type, msg.Type)
	}
	if decoded.MessageType != msg.MessageType {
		t.Errorf("Round-trip MessageType: got %q, want %q", decoded.MessageType, msg.MessageType)
	}
	if decoded.SenderID != msg.SenderID {
		t.Errorf("Round-trip SenderID: got %q, want %q", decoded.SenderID, msg.SenderID)
	}
	if decoded.MessageID != msg.MessageID {
		t.Errorf("Round-trip MessageID: got %q, want %q", decoded.MessageID, msg.MessageID)
	}
}

// ---------------------------------------------------------------------------
// 3. TestClientRegistrationMessage_JSON
// ---------------------------------------------------------------------------

func TestClientRegistrationMessage_JSON(t *testing.T) {
	msg := ClientRegistrationMessage{
		BaseMessage: NewBaseMessage(MsgTypeClientRegistration, "worker-7"),
		ClientID:    "cid-100",
		ClientType:  "worker",
		ClientName:  "my-worker",
		Version:     "1.2.3",
		Capabilities: WorkerCapabilities{
			SupportedTaskTypes: []string{"compute", "fetch"},
			MaxConcurrentTasks: 4,
		},
		AuthToken: "secret-token",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Перевіримо через map, що всі очікувані ключі присутні.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	requiredKeys := []string{
		"type", "message_type", "timestamp", "message_id", "sender_id",
		"client_id", "client_type", "client_name", "version",
		"capabilities", "auth_token",
	}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("Missing key %q in JSON", key)
		}
	}

	// Десеріалізація — capabilities.
	var decoded ClientRegistrationMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(decoded.Capabilities.SupportedTaskTypes) != 2 {
		t.Errorf("SupportedTaskTypes len: got %d, want 2", len(decoded.Capabilities.SupportedTaskTypes))
	}
	if decoded.Capabilities.MaxConcurrentTasks != 4 {
		t.Errorf("MaxConcurrentTasks: got %d, want 4", decoded.Capabilities.MaxConcurrentTasks)
	}
	if decoded.ClientID != "cid-100" {
		t.Errorf("ClientID: got %q, want %q", decoded.ClientID, "cid-100")
	}
}

// ---------------------------------------------------------------------------
// 4. TestTaskAssignmentMessage_TimeoutDuration
// ---------------------------------------------------------------------------

func TestTaskAssignmentMessage_TimeoutDuration(t *testing.T) {
	// Hub надсилає timeout у наносекундах (стандартна Go-серіалізація time.Duration).
	// 5 хвилин = 5 * 60 * 1e9 = 300_000_000_000 ns.
	payload := `{
		"type": "task",
		"message_type": "task",
		"timestamp": "2025-01-01T00:00:00Z",
		"message_id": "abc-123",
		"sender_id": "hub-1",
		"task_id": "task-42",
		"task_type": "compute",
		"timeout": 300000000000
	}`

	var msg TaskAssignmentMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	expected := 5 * time.Minute
	if msg.Timeout != expected {
		t.Errorf("Timeout: got %v, want %v", msg.Timeout, expected)
	}
	if msg.TaskID != "task-42" {
		t.Errorf("TaskID: got %q, want %q", msg.TaskID, "task-42")
	}
	if msg.TaskType != "compute" {
		t.Errorf("TaskType: got %q, want %q", msg.TaskType, "compute")
	}
}

// ---------------------------------------------------------------------------
// 5. TestTaskResultMessage_JSON
// ---------------------------------------------------------------------------

func TestTaskResultMessage_JSON(t *testing.T) {
	execTime := 2*time.Second + 500*time.Millisecond
	msg := NewTaskResultMsg("worker-1", "task-99", "completed", map[string]any{"key": "val"}, nil, execTime)

	// CompletedAt повинен бути встановлений.
	if msg.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}
	if msg.CompletedAt.IsZero() {
		t.Fatal("CompletedAt is zero")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded TaskResultMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.TaskID != "task-99" {
		t.Errorf("TaskID: got %q, want %q", decoded.TaskID, "task-99")
	}
	if decoded.Status != "completed" {
		t.Errorf("Status: got %q, want %q", decoded.Status, "completed")
	}
	if decoded.ExecutionTime != execTime {
		t.Errorf("ExecutionTime: got %v, want %v", decoded.ExecutionTime, execTime)
	}
	if decoded.CompletedAt == nil {
		t.Fatal("Decoded CompletedAt is nil")
	}
	if decoded.Type != MsgTypeTaskResult {
		t.Errorf("Type: got %q, want %q", decoded.Type, MsgTypeTaskResult)
	}
}

// ---------------------------------------------------------------------------
// 6. TestTaskResultMessage_WithError
// ---------------------------------------------------------------------------

func TestTaskResultMessage_WithError(t *testing.T) {
	taskErr := &TaskErrorInfo{
		Type:    "timeout",
		Message: "execution exceeded 5 minutes",
	}
	msg := NewTaskResultMsg("worker-1", "task-err", "failed", nil, taskErr, time.Minute)

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// JSON повинен містити поле "error".
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	errField, ok := raw["error"]
	if !ok {
		t.Fatal(`JSON does not contain "error" field`)
	}
	if string(errField) == "null" {
		t.Fatal(`"error" field is null but expected an object`)
	}

	// Десеріалізація повна.
	var decoded TaskResultMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Error == nil {
		t.Fatal("Decoded Error is nil")
	}
	if decoded.Error.Type != "timeout" {
		t.Errorf("Error.Type: got %q, want %q", decoded.Error.Type, "timeout")
	}
	if decoded.Error.Message != "execution exceeded 5 minutes" {
		t.Errorf("Error.Message: got %q, want %q", decoded.Error.Message, "execution exceeded 5 minutes")
	}
}

// ---------------------------------------------------------------------------
// 7. TestTaskResultMessage_WithoutError
// ---------------------------------------------------------------------------

func TestTaskResultMessage_WithoutError(t *testing.T) {
	msg := NewTaskResultMsg("worker-1", "task-ok", "completed", map[string]any{"v": 1}, nil, time.Second)

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Поле "error" повинно бути відсутнє або null (omitempty для вказівника).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	errField, present := raw["error"]
	if present && string(errField) != "null" {
		t.Errorf(`"error" should be null or absent, got: %s`, string(errField))
	}
}

// ---------------------------------------------------------------------------
// 8. TestPongMessage_JSON
// ---------------------------------------------------------------------------

func TestPongMessage_JSON(t *testing.T) {
	msg := NewPongMsg("worker-pong")

	if msg.Type != MsgTypePong {
		t.Errorf("Type: got %q, want %q", msg.Type, MsgTypePong)
	}
	if msg.MessageType != MsgTypePong {
		t.Errorf("MessageType: got %q, want %q", msg.MessageType, MsgTypePong)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded PongMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Type != "pong" {
		t.Errorf("Decoded Type: got %q, want %q", decoded.Type, "pong")
	}
	if decoded.MessageType != "pong" {
		t.Errorf("Decoded MessageType: got %q, want %q", decoded.MessageType, "pong")
	}
	if decoded.SenderID != "worker-pong" {
		t.Errorf("SenderID: got %q, want %q", decoded.SenderID, "worker-pong")
	}
}

// ---------------------------------------------------------------------------
// 9. TestWorkerStatusMessage_JSON
// ---------------------------------------------------------------------------

func TestWorkerStatusMessage_JSON(t *testing.T) {
	metrics := map[string]any{
		"cpu":    75.5,
		"memory": 1024.0,
	}
	msg := NewWorkerStatusMsg("sender-1", "client-1", "active", 3, 0.85, metrics)

	if msg.Type != MsgTypeWorkerStatus {
		t.Errorf("Type: got %q, want %q", msg.Type, MsgTypeWorkerStatus)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded WorkerStatusMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ClientID != "client-1" {
		t.Errorf("ClientID: got %q, want %q", decoded.ClientID, "client-1")
	}
	if decoded.Status != "active" {
		t.Errorf("Status: got %q, want %q", decoded.Status, "active")
	}
	if decoded.ActiveTasks != 3 {
		t.Errorf("ActiveTasks: got %d, want 3", decoded.ActiveTasks)
	}
	if decoded.Load != 0.85 {
		t.Errorf("Load: got %f, want 0.85", decoded.Load)
	}
	if decoded.Metrics == nil {
		t.Fatal("Metrics is nil")
	}
	if cpu, ok := decoded.Metrics["cpu"].(float64); !ok || cpu != 75.5 {
		t.Errorf("Metrics[cpu]: got %v, want 75.5", decoded.Metrics["cpu"])
	}
	if decoded.SenderID != "sender-1" {
		t.Errorf("SenderID: got %q, want %q", decoded.SenderID, "sender-1")
	}
}

// ---------------------------------------------------------------------------
// 10. TestTaskProgressMessage_JSON
// ---------------------------------------------------------------------------

func TestTaskProgressMessage_JSON(t *testing.T) {
	details := map[string]any{"step": "parsing", "records": float64(500)}
	msg := NewTaskProgressMsg("sender-p", "task-p1", 0.42, "processing", details)

	if msg.Type != MsgTypeTaskProgress {
		t.Errorf("Type: got %q, want %q", msg.Type, MsgTypeTaskProgress)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded TaskProgressMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.TaskID != "task-p1" {
		t.Errorf("TaskID: got %q, want %q", decoded.TaskID, "task-p1")
	}
	if decoded.Progress != 0.42 {
		t.Errorf("Progress: got %f, want 0.42", decoded.Progress)
	}
	if decoded.Message != "processing" {
		t.Errorf("Message: got %q, want %q", decoded.Message, "processing")
	}
	if decoded.Details == nil {
		t.Fatal("Details is nil")
	}
	if step, ok := decoded.Details["step"].(string); !ok || step != "parsing" {
		t.Errorf("Details[step]: got %v, want %q", decoded.Details["step"], "parsing")
	}
	if records, ok := decoded.Details["records"].(float64); !ok || records != 500 {
		t.Errorf("Details[records]: got %v, want 500", decoded.Details["records"])
	}
}

// ---------------------------------------------------------------------------
// 11. TestHeartbeatMessage_JSON
// ---------------------------------------------------------------------------

func TestHeartbeatMessage_JSON(t *testing.T) {
	payload := `{
		"type": "heartbeat",
		"message_type": "heartbeat",
		"timestamp": "2025-06-15T12:00:00Z",
		"message_id": "hb-001",
		"sender_id": "hub-main",
		"client_id": "worker-9",
		"metrics": {
			"uptime": 3600,
			"connected_clients": 12
		}
	}`

	var msg HeartbeatMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if msg.Type != MsgTypeHeartbeat {
		t.Errorf("Type: got %q, want %q", msg.Type, MsgTypeHeartbeat)
	}
	if msg.MessageType != "heartbeat" {
		t.Errorf("MessageType: got %q, want %q", msg.MessageType, "heartbeat")
	}
	if msg.SenderID != "hub-main" {
		t.Errorf("SenderID: got %q, want %q", msg.SenderID, "hub-main")
	}
	if msg.ClientID != "worker-9" {
		t.Errorf("ClientID: got %q, want %q", msg.ClientID, "worker-9")
	}
	if msg.Metrics == nil {
		t.Fatal("Metrics is nil")
	}
	if uptime, ok := msg.Metrics["uptime"].(float64); !ok || uptime != 3600 {
		t.Errorf("Metrics[uptime]: got %v, want 3600", msg.Metrics["uptime"])
	}
	if cc, ok := msg.Metrics["connected_clients"].(float64); !ok || cc != 12 {
		t.Errorf("Metrics[connected_clients]: got %v, want 12", msg.Metrics["connected_clients"])
	}

	expectedTS := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	if !msg.Timestamp.Equal(expectedTS) {
		t.Errorf("Timestamp: got %v, want %v", msg.Timestamp, expectedTS)
	}

	// Зворотня серіалізація.
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if !strings.Contains(string(data), `"heartbeat"`) {
		t.Error("Re-marshaled JSON does not contain heartbeat type")
	}
}

// ---------------------------------------------------------------------------
// 12. TestErrorMessage_JSON
// ---------------------------------------------------------------------------

func TestErrorMessage_JSON(t *testing.T) {
	payload := `{
		"type": "error",
		"message_type": "error",
		"timestamp": "2025-06-15T12:05:00Z",
		"message_id": "err-555",
		"sender_id": "hub-main",
		"code": "RATE_LIMIT_EXCEEDED",
		"message": "too many requests",
		"details": {
			"retry_after": 30,
			"limit": 100
		}
	}`

	var msg ErrorMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if msg.Type != MsgTypeError {
		t.Errorf("Type: got %q, want %q", msg.Type, MsgTypeError)
	}
	if msg.Code != "RATE_LIMIT_EXCEEDED" {
		t.Errorf("Code: got %q, want %q", msg.Code, "RATE_LIMIT_EXCEEDED")
	}
	if msg.Message != "too many requests" {
		t.Errorf("Message: got %q, want %q", msg.Message, "too many requests")
	}
	if msg.Details == nil {
		t.Fatal("Details is nil")
	}
	if retryAfter, ok := msg.Details["retry_after"].(float64); !ok || retryAfter != 30 {
		t.Errorf("Details[retry_after]: got %v, want 30", msg.Details["retry_after"])
	}
	if limit, ok := msg.Details["limit"].(float64); !ok || limit != 100 {
		t.Errorf("Details[limit]: got %v, want 100", msg.Details["limit"])
	}
	if msg.MessageID != "err-555" {
		t.Errorf("MessageID: got %q, want %q", msg.MessageID, "err-555")
	}

	// Зворотня серіалізація.
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ErrorMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Round-trip Unmarshal error: %v", err)
	}
	if decoded.Code != msg.Code {
		t.Errorf("Round-trip Code: got %q, want %q", decoded.Code, msg.Code)
	}
	if decoded.Message != msg.Message {
		t.Errorf("Round-trip Message: got %q, want %q", decoded.Message, msg.Message)
	}
}
