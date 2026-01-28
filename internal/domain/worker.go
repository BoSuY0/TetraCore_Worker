package domain

import "time"

// WorkerState represents the current state of the worker.
type WorkerState string

const (
	WorkerStateIdle        WorkerState = "idle"
	WorkerStateBusy        WorkerState = "busy"
	WorkerStateOverloaded  WorkerState = "overloaded"
	WorkerStateMaintenance WorkerState = "maintenance"
	WorkerStateShutdown    WorkerState = "shutdown"
	WorkerStateError       WorkerState = "error"
)

// WorkerStats holds runtime statistics for the worker process.
type WorkerStats struct {
	WorkerID       string      `json:"worker_id"`
	State          WorkerState `json:"state"`
	ActiveTasks    int         `json:"active_tasks"`
	TotalProcessed int64       `json:"total_processed"`
	TotalFailed    int64       `json:"total_failed"`
	UptimeSeconds  int64       `json:"uptime_seconds"`
	CPUUsage       *float64    `json:"cpu_usage,omitempty"`
	MemoryUsage    *float64    `json:"memory_usage,omitempty"`
	HubConnected   bool        `json:"hub_connected"`
	StartedAt      time.Time   `json:"started_at"`
}
