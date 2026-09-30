package adaptive

import (
	"time"
)

// AdaptiveSignal — агрегований сигнал для політики адаптивної конкуренції.
//
// CPU може бути NaN, якщо семплінг CPU недоступний або вимкнений.
// LatencyEWMA та ErrorEWMA — згладжені (EWMA) оцінки латентності (мс) та частки помилок (0..1).
// Samples — кількість отриманих семплів продуктивності (для прогріву/мінімальної статистики).
type AdaptiveSignal struct {
	CPU         float64
	LatencyEWMA float64
	ErrorEWMA   float64
	Samples     int
}

// SignalSource — джерело агрегованого сигналу для політики.
type SignalSource interface {
	Signal(now time.Time) (AdaptiveSignal, error)
}

// TaskMetricsCollector — збирає метрики виконання задач для подальшої агрегації.
type TaskMetricsCollector interface {
	RecordTask(total time.Duration, ok bool)
}
