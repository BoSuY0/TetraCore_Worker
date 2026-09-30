package adaptive

import (
	"fmt"
	"math"
	"time"
)

// PolicyConfig — конфігурація Hybrid AIMD політики.
type PolicyConfig struct {
	MinC             int
	MaxC             int
	TargetCPU        float64
	TargetLatencyMs  float64
	ErrorHi          float64
	ErrorLo          float64
	CPUHyst          float64
	AIStep           int
	MDFactor         float64
	CooldownUp       time.Duration
	CooldownDown     time.Duration
	HoldoffAfterDown time.Duration
	MinSamples       int
}

// HybridAIMDPolicy — гібридна політика AIMD (Additive Increase / Multiplicative Decrease),
// яка враховує CPU, латентність та частку помилок.
type HybridAIMDPolicy struct {
	cfg PolicyConfig

	lastUp       time.Time
	lastDown     time.Time
	holdoffUntil time.Time
}

// NewHybridAIMDPolicy створює політику з валідацією та безпечними дефолтами.
//
// Валідація: MinC <= MaxC.
// Clamp/fallback: AIStep, MDFactor, MinSamples.
func NewHybridAIMDPolicy(cfg PolicyConfig) (*HybridAIMDPolicy, error) {
	if cfg.MinC < 1 {
		cfg.MinC = 1
	}
	if cfg.MaxC < 1 {
		cfg.MaxC = cfg.MinC
	}
	if cfg.MinC > cfg.MaxC {
		return nil, fmt.Errorf("невалідні межі конкуренції: min=%d max=%d", cfg.MinC, cfg.MaxC)
	}

	if cfg.AIStep <= 0 {
		cfg.AIStep = 1
	}
	if cfg.MDFactor <= 0 || cfg.MDFactor >= 1 {
		cfg.MDFactor = 0.7
	}
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 1
	}
	if cfg.CPUHyst < 0 {
		cfg.CPUHyst = 0
	}
	if cfg.CooldownUp < 0 {
		cfg.CooldownUp = 0
	}
	if cfg.CooldownDown < 0 {
		cfg.CooldownDown = 0
	}
	if cfg.HoldoffAfterDown < 0 {
		cfg.HoldoffAfterDown = 0
	}

	return &HybridAIMDPolicy{cfg: cfg}, nil
}

// Next ухвалює рішення про наступну конкуренцію.
//
// Reason (рівно за ТЗ):
// - decrease: "latency" | "error" | "cpu"
// - increase: "low_load"
// - skip: "cooldown" | "holdoff" | "min_samples"
// - else: "no_change"
func (p *HybridAIMDPolicy) Next(cur int, sig AdaptiveSignal, now time.Time) (next int, reason string, changed bool) {
	cur = clampInt(cur, p.cfg.MinC, p.cfg.MaxC)

	if sig.Samples < p.cfg.MinSamples {
		return cur, "min_samples", false
	}

	// 1) Congestion → decrease.
	if decReason := p.congestionReason(sig); decReason != "" {
		if !p.lastDown.IsZero() && p.cfg.CooldownDown > 0 && now.Sub(p.lastDown) < p.cfg.CooldownDown {
			return cur, "cooldown", false
		}

		candidate := int(math.Floor(float64(cur) * p.cfg.MDFactor))
		if candidate >= cur {
			candidate = cur - 1
		}
		next = clampInt(candidate, p.cfg.MinC, p.cfg.MaxC)
		if next == cur {
			return cur, "no_change", false
		}

		p.lastDown = now
		if p.cfg.HoldoffAfterDown > 0 {
			p.holdoffUntil = now.Add(p.cfg.HoldoffAfterDown)
		} else {
			p.holdoffUntil = time.Time{}
		}

		return next, decReason, true
	}

	// 2) Underutilized → increase.
	if p.underutilized(sig) {
		if !p.holdoffUntil.IsZero() && now.Before(p.holdoffUntil) {
			return cur, "holdoff", false
		}
		if !p.lastUp.IsZero() && p.cfg.CooldownUp > 0 && now.Sub(p.lastUp) < p.cfg.CooldownUp {
			return cur, "cooldown", false
		}

		candidate := cur + p.cfg.AIStep
		next = clampInt(candidate, p.cfg.MinC, p.cfg.MaxC)
		if next == cur {
			return cur, "no_change", false
		}

		p.lastUp = now
		return next, "low_load", true
	}

	return cur, "no_change", false
}

func (p *HybridAIMDPolicy) congestionReason(sig AdaptiveSignal) string {
	// Пріоритет: error → latency → cpu.
	if p.cfg.ErrorHi > 0 && sig.ErrorEWMA >= p.cfg.ErrorHi {
		return "error"
	}
	if p.cfg.TargetLatencyMs > 0 && sig.LatencyEWMA > p.cfg.TargetLatencyMs*1.2 {
		return "latency"
	}
	if !math.IsNaN(sig.CPU) && p.cfg.TargetCPU > 0 && sig.CPU > (p.cfg.TargetCPU+p.cfg.CPUHyst) {
		return "cpu"
	}
	return ""
}

func (p *HybridAIMDPolicy) underutilized(sig AdaptiveSignal) bool {
	// Без достатньої статистики (LatencyEWMA==0) підвищення може бути шумовим.
	// Але якщо TargetLatencyMs не заданий — не блокуємо підвищення по цій ознаці.
	if p.cfg.TargetLatencyMs > 0 {
		if sig.LatencyEWMA <= 0 || sig.LatencyEWMA >= p.cfg.TargetLatencyMs {
			return false
		}
	}
	if p.cfg.ErrorLo > 0 && sig.ErrorEWMA >= p.cfg.ErrorLo {
		return false
	}

	if math.IsNaN(sig.CPU) {
		// CPU недоступний: дозволяємо increase лише за "хороших" latency/error.
		return true
	}

	if p.cfg.TargetCPU > 0 {
		if sig.CPU >= (p.cfg.TargetCPU - p.cfg.CPUHyst) {
			return false
		}
	}

	return true
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
