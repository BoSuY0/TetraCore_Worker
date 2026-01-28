package domain

import (
	"testing"
)

// ---------------------------------------------------------------------------
// TestExtractActionParams_NestedTaskData — payload з вкладеною структурою
// task_data. Перевіряємо, що Action і Params витягуються коректно.
// ---------------------------------------------------------------------------
func TestExtractActionParams_NestedTaskData(t *testing.T) {
	msg := &TaskMessage{
		TaskID: "task-1",
		Payload: map[string]any{
			"task_data": map[string]any{
				"action": "test",
				"params": map[string]any{
					"key": "val",
				},
			},
		},
	}

	ap := msg.ExtractActionParams()

	if ap.TaskID != "task-1" {
		t.Errorf("TaskID: got %q, want %q", ap.TaskID, "task-1")
	}
	if ap.Action != "test" {
		t.Errorf("Action: got %q, want %q", ap.Action, "test")
	}
	if v, ok := ap.Params["key"]; !ok || v != "val" {
		t.Errorf("Params[\"key\"]: got %v (ok=%v), want \"val\"", v, ok)
	}
}

// ---------------------------------------------------------------------------
// TestExtractActionParams_FlatLayout — fallback: дані лежать прямо в payload.
// ---------------------------------------------------------------------------
func TestExtractActionParams_FlatLayout(t *testing.T) {
	msg := &TaskMessage{
		TaskID: "task-2",
		Payload: map[string]any{
			"action": "test",
			"params": map[string]any{
				"key": "val",
			},
		},
	}

	ap := msg.ExtractActionParams()

	if ap.Action != "test" {
		t.Errorf("Action: got %q, want %q", ap.Action, "test")
	}
	if v, ok := ap.Params["key"]; !ok || v != "val" {
		t.Errorf("Params[\"key\"]: got %v (ok=%v), want \"val\"", v, ok)
	}
}

// ---------------------------------------------------------------------------
// TestExtractActionParams_MissingAction — порожній payload, Action має бути "".
// ---------------------------------------------------------------------------
func TestExtractActionParams_MissingAction(t *testing.T) {
	msg := &TaskMessage{
		TaskID:  "task-3",
		Payload: map[string]any{},
	}

	ap := msg.ExtractActionParams()

	if ap.Action != "" {
		t.Errorf("Action: got %q, want empty string", ap.Action)
	}
}

// ---------------------------------------------------------------------------
// TestExtractActionParams_ChatID — chat_id витягується як *int64.
// ---------------------------------------------------------------------------
func TestExtractActionParams_ChatID(t *testing.T) {
	msg := &TaskMessage{
		TaskID: "task-4",
		Payload: map[string]any{
			"task_data": map[string]any{
				"action": "test",
				"params": map[string]any{
					"chat_id": float64(12345),
				},
			},
		},
	}

	ap := msg.ExtractActionParams()

	if ap.ChatID == nil {
		t.Fatal("ChatID: got nil, want *int64(12345)")
	}
	if *ap.ChatID != 12345 {
		t.Errorf("ChatID: got %d, want 12345", *ap.ChatID)
	}
}

// ---------------------------------------------------------------------------
// TestExtractActionParams_UserID — user_id витягується як *int64.
// ---------------------------------------------------------------------------
func TestExtractActionParams_UserID(t *testing.T) {
	msg := &TaskMessage{
		TaskID: "task-5",
		Payload: map[string]any{
			"task_data": map[string]any{
				"action": "test",
				"params": map[string]any{
					"user_id": float64(67890),
				},
			},
		},
	}

	ap := msg.ExtractActionParams()

	if ap.UserID == nil {
		t.Fatal("UserID: got nil, want *int64(67890)")
	}
	if *ap.UserID != 67890 {
		t.Errorf("UserID: got %d, want 67890", *ap.UserID)
	}
}

// ---------------------------------------------------------------------------
// TestExtractActionParams_ChatIDFloat64 — JSON числа десеріалізуються як
// float64. Перевіряємо коректну конвертацію у int64.
// ---------------------------------------------------------------------------
func TestExtractActionParams_ChatIDFloat64(t *testing.T) {
	msg := &TaskMessage{
		TaskID: "task-6",
		Payload: map[string]any{
			"task_data": map[string]any{
				"action": "test",
				"params": map[string]any{
					"chat_id": float64(99999),
				},
			},
		},
	}

	ap := msg.ExtractActionParams()

	if ap.ChatID == nil {
		t.Fatal("ChatID: got nil, want *int64(99999)")
	}
	if *ap.ChatID != 99999 {
		t.Errorf("ChatID: got %d, want 99999", *ap.ChatID)
	}
}

// ---------------------------------------------------------------------------
// TestExtractActionParams_NoParams — є action, але немає params.
// Params має бути порожньою мапою (не nil).
// ---------------------------------------------------------------------------
func TestExtractActionParams_NoParams(t *testing.T) {
	msg := &TaskMessage{
		TaskID: "task-7",
		Payload: map[string]any{
			"task_data": map[string]any{
				"action": "test",
			},
		},
	}

	ap := msg.ExtractActionParams()

	if ap.Action != "test" {
		t.Errorf("Action: got %q, want %q", ap.Action, "test")
	}
	if ap.Params == nil {
		t.Fatal("Params: got nil, want empty map")
	}
	if len(ap.Params) != 0 {
		t.Errorf("Params length: got %d, want 0", len(ap.Params))
	}
}

// ---------------------------------------------------------------------------
// TestNewSuccessResult — Success=true, Data встановлено, Error == nil.
// ---------------------------------------------------------------------------
func TestNewSuccessResult(t *testing.T) {
	data := map[string]any{"foo": "bar"}
	result := NewSuccessResult(data)

	if !result.Success {
		t.Error("Success: got false, want true")
	}
	if result.Data == nil {
		t.Fatal("Data: got nil, want non-nil map")
	}
	if v, ok := result.Data["foo"]; !ok || v != "bar" {
		t.Errorf("Data[\"foo\"]: got %v (ok=%v), want \"bar\"", v, ok)
	}
	if result.Error != nil {
		t.Errorf("Error: got %+v, want nil", result.Error)
	}
}

// ---------------------------------------------------------------------------
// TestNewErrorResult — Success=false, Error.Code та Error.Message заповнені.
// ---------------------------------------------------------------------------
func TestNewErrorResult(t *testing.T) {
	result := NewErrorResult("ERR_CODE", "something went wrong")

	if result.Success {
		t.Error("Success: got true, want false")
	}
	if result.Error == nil {
		t.Fatal("Error: got nil, want non-nil ActionError")
	}
	if result.Error.Code != "ERR_CODE" {
		t.Errorf("Error.Code: got %q, want %q", result.Error.Code, "ERR_CODE")
	}
	if result.Error.Message != "something went wrong" {
		t.Errorf("Error.Message: got %q, want %q", result.Error.Message, "something went wrong")
	}
	if result.Data != nil {
		t.Errorf("Data: got %v, want nil", result.Data)
	}
}

// ---------------------------------------------------------------------------
// TestTaskStatus_Constants — перевіряємо рядкові значення всіх TaskStatus.
// ---------------------------------------------------------------------------
func TestTaskStatus_Constants(t *testing.T) {
	cases := []struct {
		got  TaskStatus
		want string
	}{
		{TaskStatusPending, "pending"},
		{TaskStatusAssigned, "assigned"},
		{TaskStatusProcessing, "processing"},
		{TaskStatusCompleted, "completed"},
		{TaskStatusFailed, "failed"},
		{TaskStatusTimeout, "timeout"},
		{TaskStatusCancelled, "cancelled"},
	}

	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("TaskStatus: got %q, want %q", string(tc.got), tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestTaskPriority_Constants — перевіряємо рядкові значення всіх TaskPriority.
// ---------------------------------------------------------------------------
func TestTaskPriority_Constants(t *testing.T) {
	cases := []struct {
		got  TaskPriority
		want string
	}{
		{TaskPriorityLow, "low"},
		{TaskPriorityNormal, "normal"},
		{TaskPriorityHigh, "high"},
		{TaskPriorityCritical, "critical"},
	}

	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("TaskPriority: got %q, want %q", string(tc.got), tc.want)
		}
	}
}
