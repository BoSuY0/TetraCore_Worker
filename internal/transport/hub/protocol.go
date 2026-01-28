package hub

import (
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// MessageType — константи типів повідомлень WebSocket-протоколу Hub.
// ---------------------------------------------------------------------------

const (
	// Типи повідомлень клієнт -> Hub.
	MsgTypeClientRegistration MessageType = "client_registration"
	MsgTypeTaskResult         MessageType = "task_result"
	MsgTypePong               MessageType = "pong"
	MsgTypeWorkerStatus       MessageType = "worker_status"
	MsgTypeTaskProgress       MessageType = "task_progress"

	// Типи повідомлень Hub -> клієнт.
	// Примітка: Hub відповідає на реєстрацію повідомленням з type "client_registration" (той самий тип).
	// Окремого "registration_response" типу не існує.
	MsgTypeTask       MessageType = "task"
	MsgTypeHeartbeat  MessageType = "heartbeat"
	MsgTypeTaskCancel MessageType = "task_cancel"
	MsgTypeError      MessageType = "error"
)

// MessageType — рядковий тип повідомлення протоколу.
type MessageType = string

// ---------------------------------------------------------------------------
// BaseMessage — базова структура для всіх повідомлень протоколу.
// ---------------------------------------------------------------------------

// BaseMessage містить загальні поля, присутні в кожному повідомленні.
// Timestamp — time.Time для сумісності з Hub (серіалізується як RFC3339).
type BaseMessage struct {
	Type        MessageType `json:"type"`
	MessageType string      `json:"message_type,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
	MessageID   string      `json:"message_id,omitempty"`
	SenderID    string      `json:"sender_id,omitempty"`
}

// NewBaseMessage створює BaseMessage з заданим типом і sender_id.
func NewBaseMessage(msgType MessageType, senderID string) BaseMessage {
	return BaseMessage{
		Type:        msgType,
		MessageType: string(msgType),
		Timestamp:   time.Now().UTC(),
		MessageID:   uuid.New().String(),
		SenderID:    senderID,
	}
}

// ---------------------------------------------------------------------------
// Registration (клієнт -> Hub).
// ---------------------------------------------------------------------------

// ClientRegistrationMessage — повідомлення реєстрації воркера на Hub.
type ClientRegistrationMessage struct {
	BaseMessage
	ClientID     string             `json:"client_id"`
	ClientType   string             `json:"client_type"`
	ClientName   string             `json:"client_name,omitempty"`
	Version      string             `json:"version"`
	Capabilities WorkerCapabilities `json:"capabilities,omitempty"`
	AuthToken    string             `json:"auth_token,omitempty"`
}

// ClientRegistrationResponse — відповідь Hub на реєстрацію.
type ClientRegistrationResponse struct {
	BaseMessage
	Success   bool   `json:"success"`
	ClientID  string `json:"client_id,omitempty"`
	Message   string `json:"message,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// WorkerCapabilities — можливості воркера, повідомлені при реєстрації.
// Поля узгоджені з Hub entity.WorkerCapabilities.
type WorkerCapabilities struct {
	SupportedTaskTypes    []string `json:"supported_task_types,omitempty"`
	MaxConcurrentTasks    int      `json:"max_concurrent_tasks,omitempty"`
	AverageProcessingTime float64  `json:"average_processing_time,omitempty"`
	MaxTaskSize           int64    `json:"max_task_size,omitempty"`
	SupportedFormats      []string `json:"supported_formats,omitempty"`
	SpecialCapabilities   []string `json:"special_capabilities,omitempty"`
	APIVersion            string   `json:"api_version,omitempty"`
	MinPriority           string   `json:"min_priority,omitempty"`
	MaxPriority           string   `json:"max_priority,omitempty"`
	// Додаткові поля Worker (Hub ігнорує, але корисні для логування).
	Version string `json:"version,omitempty"`
	OS      string `json:"os,omitempty"`
	Arch    string `json:"arch,omitempty"`
}

// ---------------------------------------------------------------------------
// Task (Hub -> клієнт).
// ---------------------------------------------------------------------------

// TaskAssignmentMessage — завдання, призначене Hub для воркера.
type TaskAssignmentMessage struct {
	BaseMessage
	TaskID       string         `json:"task_id"`
	TaskType     string         `json:"task_type"`
	ExecutorType string         `json:"executor_type,omitempty"`
	Priority     string         `json:"priority,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
	Timeout      time.Duration  `json:"timeout,omitempty"` // time.Duration від Hub (наносекунди)
	RetryCount   int            `json:"retry_count,omitempty"`
}

// TaskCancelMessage — Hub скасовує завдання.
type TaskCancelMessage struct {
	BaseMessage
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Task Result / Progress (клієнт -> Hub).
// ---------------------------------------------------------------------------

// TaskResultMessage — результат виконання завдання, надісланий Hub.
type TaskResultMessage struct {
	BaseMessage
	TaskID        string         `json:"task_id"`
	Status        string         `json:"status"`
	Result        map[string]any `json:"result,omitempty"`
	Error         *TaskErrorInfo `json:"error,omitempty"`
	ExecutionTime time.Duration  `json:"execution_time,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
}

// TaskErrorInfo — деталі помилки у результаті завдання.
type TaskErrorInfo struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

// TaskProgressMessage — повідомлення про прогрес виконання.
type TaskProgressMessage struct {
	BaseMessage
	TaskID   string         `json:"task_id"`
	Progress float64        `json:"progress"` // 0.0 – 1.0
	Message  string         `json:"message,omitempty"`
	Details  map[string]any `json:"details,omitempty"`
}

// ---------------------------------------------------------------------------
// Heartbeat / Pong (Hub <-> клієнт).
// ---------------------------------------------------------------------------

// HeartbeatMessage — пінг від Hub (клієнт відповідає PongMessage).
type HeartbeatMessage struct {
	BaseMessage
	ClientID string         `json:"client_id,omitempty"`
	Metrics  map[string]any `json:"metrics,omitempty"`
}

// PongMessage — відповідь клієнта на heartbeat.
type PongMessage struct {
	BaseMessage
}

// ---------------------------------------------------------------------------
// Worker Status (клієнт -> Hub).
// ---------------------------------------------------------------------------

// WorkerStatusMessage — статус воркера, що надсилається Hub періодично.
type WorkerStatusMessage struct {
	BaseMessage
	ClientID    string         `json:"client_id"`
	Status      string         `json:"status"`
	ActiveTasks int            `json:"active_tasks"`
	Load        float64        `json:"load,omitempty"`
	Metrics     map[string]any `json:"metrics,omitempty"`
}

// ---------------------------------------------------------------------------
// Error (Hub -> клієнт).
// ---------------------------------------------------------------------------

// ErrorMessage — повідомлення про помилку від Hub.
type ErrorMessage struct {
	BaseMessage
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ---------------------------------------------------------------------------
// Helper Constructors.
// ---------------------------------------------------------------------------

// NewTaskResultMsg створює TaskResultMessage на основі domain.TaskResult.
func NewTaskResultMsg(senderID, taskID, status string, result map[string]any, taskErr *TaskErrorInfo, execTime time.Duration) *TaskResultMessage {
	now := time.Now().UTC()
	return &TaskResultMessage{
		BaseMessage:   NewBaseMessage(MsgTypeTaskResult, senderID),
		TaskID:        taskID,
		Status:        status,
		Result:        result,
		Error:         taskErr,
		ExecutionTime: execTime,
		CompletedAt:   &now,
	}
}

// NewPongMsg створює PongMessage.
func NewPongMsg(senderID string) *PongMessage {
	return &PongMessage{
		BaseMessage: NewBaseMessage(MsgTypePong, senderID),
	}
}

// NewWorkerStatusMsg створює WorkerStatusMessage.
func NewWorkerStatusMsg(senderID, clientID, status string, activeTasks int, load float64, metrics map[string]any) *WorkerStatusMessage {
	return &WorkerStatusMessage{
		BaseMessage: NewBaseMessage(MsgTypeWorkerStatus, senderID),
		ClientID:    clientID,
		Status:      status,
		ActiveTasks: activeTasks,
		Load:        load,
		Metrics:     metrics,
	}
}

// NewTaskProgressMsg створює TaskProgressMessage.
func NewTaskProgressMsg(senderID, taskID string, progress float64, message string, details map[string]any) *TaskProgressMessage {
	return &TaskProgressMessage{
		BaseMessage: NewBaseMessage(MsgTypeTaskProgress, senderID),
		TaskID:      taskID,
		Progress:    progress,
		Message:     message,
		Details:     details,
	}
}
