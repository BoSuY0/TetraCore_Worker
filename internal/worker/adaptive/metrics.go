package adaptive

import (
	"sync"
	"time"
)

// EWMAMetrics — потокобезпечний колектор EWMA-метрик для latency/error.
type EWMAMetrics struct {
	mu sync.Mutex

	alpha      float64
	minSamples int

	latencyEWMA float64
	errorEWMA   float64
	samples     int

	latencyInit bool
	errorInit   bool
}

// EWMASnapshot — знімок EWMA-метрик та стану прогріву.
type EWMASnapshot struct {
	LatencyEWMA float64
	ErrorEWMA   float64
	Samples     int
	MinSamples  int
	Ready       bool
}

// NewEWMAMetrics створює колектор з валідацією параметрів.
func NewEWMAMetrics(alpha float64, minSamples int) *EWMAMetrics {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.2
	}
	if minSamples < 1 {
		minSamples = 1
	}
	return &EWMAMetrics{
		alpha:      alpha,
		minSamples: minSamples,
	}
}

// Record додає семпл latency/error та оновлює EWMA.
// latencyMs <= 0 пропускає оновлення latencyEWMA, але семпл все одно рахується.
func (m *EWMAMetrics) Record(latencyMs float64, isError bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.samples++

	if latencyMs > 0 {
		if !m.latencyInit {
			m.latencyEWMA = latencyMs
			m.latencyInit = true
		} else {
			m.latencyEWMA = m.latencyEWMA*(1-m.alpha) + latencyMs*m.alpha
		}
	}

	errSample := 0.0
	if isError {
		errSample = 1.0
	}
	if !m.errorInit {
		m.errorEWMA = errSample
		m.errorInit = true
	} else {
		m.errorEWMA = m.errorEWMA*(1-m.alpha) + errSample*m.alpha
	}
}

// RecordTask — адаптер під інтерфейс TaskMetricsCollector.
func (m *EWMAMetrics) RecordTask(total time.Duration, ok bool) {
	if m == nil {
		return
	}
	latencyMs := float64(total) / float64(time.Millisecond)
	m.Record(latencyMs, !ok)
}

// Snapshot повертає потокобезпечний знімок метрик.
func (m *EWMAMetrics) Snapshot() EWMASnapshot {
	if m == nil {
		return EWMASnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	ready := m.samples >= m.minSamples
	return EWMASnapshot{
		LatencyEWMA: m.latencyEWMA,
		ErrorEWMA:   m.errorEWMA,
		Samples:     m.samples,
		MinSamples:  m.minSamples,
		Ready:       ready,
	}
}
