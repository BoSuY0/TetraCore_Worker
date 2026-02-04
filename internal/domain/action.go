package domain

import "context"

// ActionHandler defines the interface for task action handlers.
// Кожен action (наприклад "get_chat_settings") реалізує цей інтерфейс.
type ActionHandler interface {
	// Execute runs the action with given parameters and returns a result.
	Execute(ctx context.Context, params ActionParams) (ActionResult, error)
	// Name returns the unique action identifier (e.g. "get_chat_settings").
	Name() string
}

// ActionParams holds the input parameters for an action.
type ActionParams struct {
	TaskID string         `json:"task_id"`
	Action string         `json:"action"`
	ChatID *int64         `json:"chat_id,omitempty"`
	UserID *int64         `json:"user_id,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// ActionResult holds the output of an action execution.
type ActionResult struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data,omitempty"`
	Error   *ActionError   `json:"error,omitempty"`
}

// ActionError represents a structured error from an action.
type ActionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewSuccessResult creates a successful ActionResult.
func NewSuccessResult(data map[string]any) ActionResult {
	return ActionResult{Success: true, Data: data}
}

// NewErrorResult creates a failed ActionResult.
func NewErrorResult(code, message string) ActionResult {
	return ActionResult{
		Success: false,
		Error:   &ActionError{Code: code, Message: message},
	}
}
