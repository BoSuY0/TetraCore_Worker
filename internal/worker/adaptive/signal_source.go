package adaptive

import (
	"math"
	"sync"
	"time"
)

// EWMASignalSource агрегує CPU + EWMA latency/error у сигнал для політики.
type EWMASignalSource struct {
	mu sync.Mutex

	metrics  *EWMAMetrics
	cpuAlpha float64

	cpuEWMA      float64
	cpuAvailable bool
	prevTimes    CPUTimes
	prevInit     bool
}

// NewEWMASignalSource створює джерело сигналу з EWMA CPU.
func NewEWMASignalSource(metrics *EWMAMetrics, cpuAlpha float64) *EWMASignalSource {
	if cpuAlpha <= 0 || cpuAlpha > 1 {
		cpuAlpha = 0.2
	}
	return &EWMASignalSource{
		metrics:  metrics,
		cpuAlpha: cpuAlpha,
		cpuEWMA:  math.NaN(),
	}
}

// Signal повертає агрегований сигнал (CPU/latency/error).
func (s *EWMASignalSource) Signal(_ time.Time) (AdaptiveSignal, error) {
	snap := EWMASnapshot{}
	if s.metrics != nil {
		snap = s.metrics.Snapshot()
	}

	cpu := math.NaN()

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cpuAvailable {
		if t, err := ReadCPUTimes(); err == nil {
			s.prevTimes = t
			s.prevInit = true
			s.cpuAvailable = true
		}
	}

	if s.cpuAvailable {
		cur, err := ReadCPUTimes()
		if err != nil {
			s.cpuAvailable = false
		} else {
			if s.prevInit {
				if usage, ok := CPUUsage(s.prevTimes, cur); ok {
					if math.IsNaN(s.cpuEWMA) {
						s.cpuEWMA = usage
					} else {
						s.cpuEWMA = s.cpuEWMA*(1-s.cpuAlpha) + usage*s.cpuAlpha
					}
				}
			}
			s.prevTimes = cur
			s.prevInit = true
		}
	}

	if !math.IsNaN(s.cpuEWMA) {
		cpu = s.cpuEWMA
	}

	return AdaptiveSignal{
		CPU:         cpu,
		LatencyEWMA: snap.LatencyEWMA,
		ErrorEWMA:   snap.ErrorEWMA,
		Samples:     snap.Samples,
	}, nil
}
