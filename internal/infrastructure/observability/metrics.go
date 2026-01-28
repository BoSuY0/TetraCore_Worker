package observability

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// Metrics — набір Prometheus-метрик для моніторингу воркера.
type Metrics struct {
	// TasksProcessedTotal — лічильник успішно оброблених задач (label: action).
	TasksProcessedTotal *prometheus.CounterVec

	// TasksFailedTotal — лічильник невдалих задач (labels: action, error_code).
	TasksFailedTotal *prometheus.CounterVec

	// TaskDurationSeconds — гістограма тривалості виконання задач (label: action).
	TaskDurationSeconds *prometheus.HistogramVec

	// ActiveTasks — поточна кількість задач, що виконуються.
	ActiveTasks prometheus.Gauge

	// HubConnected — стан підключення до Hub (1 = підключено, 0 = ні).
	HubConnected prometheus.Gauge

	// WorkerUptimeSeconds — час роботи воркера в секундах.
	WorkerUptimeSeconds prometheus.Gauge
}

// NewMetrics створює та реєструє набір Prometheus-метрик
// з вказаним namespace (зазвичай "tetracore_worker").
func NewMetrics(namespace string) *Metrics {
	m := &Metrics{
		TasksProcessedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tasks_processed_total",
				Help:      "Total number of successfully processed tasks",
			},
			[]string{"action"},
		),

		TasksFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tasks_failed_total",
				Help:      "Total number of failed tasks",
			},
			[]string{"action", "error_code"},
		),

		TaskDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "task_duration_seconds",
				Help:      "Duration of task execution in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"action"},
		),

		ActiveTasks: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_tasks",
				Help:      "Number of currently active (in-progress) tasks",
			},
		),

		HubConnected: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "hub_connected",
				Help:      "Whether the worker is connected to Hub (1=yes, 0=no)",
			},
		),

		WorkerUptimeSeconds: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "worker_uptime_seconds",
				Help:      "Worker uptime in seconds",
			},
		),
	}

	// Реєстрація метрик у глобальному реєстрі Prometheus.
	prometheus.MustRegister(
		m.TasksProcessedTotal,
		m.TasksFailedTotal,
		m.TaskDurationSeconds,
		m.ActiveTasks,
		m.HubConnected,
		m.WorkerUptimeSeconds,
	)

	return m
}

// ServeMetrics запускає HTTP-сервер для віддачі метрик Prometheus
// на вказаному порту (endpoint /metrics).
// Ця функція блокуюча — очікується виклик у горутині.
func ServeMetrics(port int, logger zerolog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", port)
	logger.Info().Str("addr", addr).Msg("starting metrics server")

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error().Err(err).Msg("metrics server error")
	}
}
