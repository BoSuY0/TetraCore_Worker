package adaptive

import (
	"fmt"
	"math"
	"time"
)

// CPUOnlyConfig — конфігурація CPU-only політики.
type CPUOnlyConfig struct {
	MinC     int
	MaxC     int
	Target   int
	Hyst     int
	Cooldown time.Duration
	MinHold  time.Duration
}

// CPUOnlyPolicy — проста політика на основі CPU.
type CPUOnlyPolicy struct {
	cfg        CPUOnlyConfig
	lastChange time.Time
}

// NewCPUOnlyPolicy створює CPU-only політику з нормалізацією параметрів.
func NewCPUOnlyPolicy(cfg CPUOnlyConfig) *CPUOnlyPolicy {
	if cfg.MinC < 1 {
		cfg.MinC = 1
	}
	if cfg.MaxC < cfg.MinC {
		cfg.MaxC = cfg.MinC
	}
	if cfg.Target <= 0 {
		cfg.Target = 70
	}
	if cfg.Hyst <= 0 {
		cfg.Hyst = 10
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 1 * time.Second
	}
	if cfg.MinHold <= 0 {
		cfg.MinHold = 1 * time.Second
	}
	return &CPUOnlyPolicy{cfg: cfg}
}

// Next ухвалює рішення на основі CPU сигналу.
func (p *CPUOnlyPolicy) Next(cur int, sig AdaptiveSignal, now time.Time) (next int, reason string, changed bool) {
	cur = clampInt(cur, p.cfg.MinC, p.cfg.MaxC)

	// CPU-only політика без CPU не ухвалює рішень (деградація без паніки).
	if math.IsNaN(sig.CPU) {
		return cur, "no_change", false
	}

	// Мінімальна пауза між будь-якими змінами (anti-flap).
	if !p.lastChange.IsZero() && now.Sub(p.lastChange) < p.cfg.MinHold {
		return cur, "no_change", false
	}

	next = cur
	if sig.CPU > float64(p.cfg.Target+p.cfg.Hyst) {
		next = cur - 1
	} else if sig.CPU < float64(p.cfg.Target-p.cfg.Hyst) {
		next = cur + 1
	}
	next = clampInt(next, p.cfg.MinC, p.cfg.MaxC)
	if next == cur {
		return cur, "no_change", false
	}

	// Додатковий захист від флатера.
	if !p.lastChange.IsZero() && now.Sub(p.lastChange) < p.cfg.Cooldown {
		return cur, "no_change", false
	}

	p.lastChange = now
	return next, fmt.Sprintf("cpu=%.0f%% target=%d±%d", sig.CPU, p.cfg.Target, p.cfg.Hyst), true
}
