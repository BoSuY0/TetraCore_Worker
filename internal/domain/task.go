package domain

import "time"

// TaskPriority represents task priority levels (mirrors Hub).
type TaskPriority string

const (
	TaskPriorityLow      TaskPriority = "low"
	TaskPriorityNormal   TaskPriority = "normal"
	TaskPriorityHigh     TaskPriority = "high"
	TaskPriorityCritical TaskPriority = "critical"
)

// TaskStatus represents task execution status (mirrors Hub).
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusAssigned   TaskStatus = "assigned"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusTimeout    TaskStatus = "timeout"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// TaskMessage represents a task received from Hub via WebSocket.
type TaskMessage struct {
	TaskID       string         `json:"task_id"`
	TaskType     string         `json:"task_type"`
	ExecutorType string         `json:"executor_type"`
	Priority     TaskPriority   `json:"priority"`
	Payload      map[string]any `json:"payload"`
	Timeout      time.Duration  `json:"timeout"`
	RetryCount   int            `json:"retry_count,omitempty"`
}

// TaskResult represents the result sent back to Hub.
type TaskResult struct {
	TaskID        string         `json:"task_id"`
	Status        TaskStatus     `json:"status"`
	Result        map[string]any `json:"result,omitempty"`
	Error         *TaskError     `json:"error,omitempty"`
	ExecutionTime time.Duration  `json:"execution_time,omitempty"`
}

// TaskError represents an error in task execution.
type TaskError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ExtractActionParams extracts ActionParams from a TaskMessage.
// It supports both flat payload layout and nested "task_data" layout
// to stay compatible with Hub's entity/message.go format.
func (t *TaskMessage) ExtractActionParams() ActionParams {
	params := ActionParams{
		TaskID: t.TaskID,
		Params: make(map[string]any),
	}

	// Try nested task_data first (Hub's standard format).
	if td, ok := t.Payload["task_data"].(map[string]any); ok {
		if action, ok := td["action"].(string); ok {
			params.Action = action
		}
		if p, ok := td["params"].(map[string]any); ok {
			params.Params = p
		}
	} else {
		// Fallback: flat payload layout.
		if action, ok := t.Payload["action"].(string); ok {
			params.Action = action
		}
		if p, ok := t.Payload["params"].(map[string]any); ok {
			params.Params = p
		}
	}

	// Extract chat_id.
	if chatID, ok := extractInt64(params.Params, "chat_id"); ok {
		params.ChatID = &chatID
	}
	// Extract user_id.
	if userID, ok := extractInt64(params.Params, "user_id"); ok {
		params.UserID = &userID
	}

	return params
}

// extractInt64 safely extracts an int64 value from a generic map.
// JSON numbers are typically decoded as float64, so we handle that case.
func extractInt64(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case int64:
		return val, true
	case int:
		return int64(val), true
	default:
		return 0, false
	}
}
