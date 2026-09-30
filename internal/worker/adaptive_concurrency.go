package worker

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/BoSuY0/tetracore-worker/internal/worker/adaptive"
)

type taskMetricsProvider interface {
	TaskMetricsCollector() adaptive.TaskMetricsCollector
}

func (w *Worker) initAdaptiveMetrics() {
	if w.adaptiveEWMA != nil {
		return
	}

	minSamples := w.config.Worker.AdaptiveMinSamples
	if minSamples <= 0 {
		minSamples = 20
	}

	// EWMA alpha поки що фіксований; за потреби можна винести в конфіг.
	w.adaptiveEWMA = adaptive.NewEWMAMetrics(0.2, minSamples)

	if w.metrics != nil {
		if provider, ok := any(w.metrics).(taskMetricsProvider); ok {
			w.taskMetrics = provider.TaskMetricsCollector()
		}
	}
}

func (w *Worker) recordAdaptiveMetrics(total time.Duration, ok bool) {
	if w.adaptiveEWMA != nil {
		w.adaptiveEWMA.RecordTask(total, ok)
	}
	if w.taskMetrics != nil {
		w.taskMetrics.RecordTask(total, ok)
	}
}

func (w *Worker) runAdaptiveConcurrency(ctx context.Context) {
	cfg := w.config.Worker

	policyName := strings.TrimSpace(strings.ToLower(cfg.AdaptivePolicy))
	switch policyName {
	case "", "cpu", "cpu_only":
		policyName = "cpu_only"
	case "hybrid_aimd":
		// ok
	default:
		w.logger.Warn().Str("adaptive_policy", policyName).Msg("невідома adaptive_policy, fallback на cpu_only")
		policyName = "cpu_only"
	}

	poll := time.Duration(cfg.AdaptivePollSec * float64(time.Second))
	if poll <= 0 {
		poll = 1 * time.Second
	}

	cooldownLegacy := time.Duration(cfg.AdaptiveCooldownSec * float64(time.Second))
	if cooldownLegacy <= 0 {
		cooldownLegacy = 1 * time.Second
	}
	cooldownUp := time.Duration(cfg.AdaptiveCooldownUpSec * float64(time.Second))
	cooldownDown := time.Duration(cfg.AdaptiveCooldownDownSec * float64(time.Second))
	if cooldownUp <= 0 {
		cooldownUp = cooldownLegacy
	}
	if cooldownDown <= 0 {
		cooldownDown = cooldownLegacy
	}
	holdoffAfterDown := time.Duration(cfg.AdaptiveHoldoffAfterDownSec * float64(time.Second))
	if holdoffAfterDown < 0 {
		holdoffAfterDown = 0
	}

	minC := cfg.AdaptiveMinConcurrency
	if minC < 1 {
		minC = 1
	}
	maxC := cfg.AdaptiveMaxConcurrency
	if maxC < minC {
		maxC = minC
	}
	if execMax := w.executor.MaxConcurrency(); execMax > 0 && maxC > execMax {
		w.logger.Warn().Int("adaptive_max", maxC).Int("executor_max", execMax).Msg("adaptive_max_concurrency перевищує місткість executor — використано executor_max")
		maxC = execMax
	}

	cur := w.executor.Concurrency()
	if cur < minC {
		cur = minC
	}
	if cur > maxC {
		cur = maxC
	}
	if applied := w.executor.SetConcurrency(cur); applied != cur {
		cur = applied
	}

	minSamples := cfg.AdaptiveMinSamples
	if minSamples <= 0 {
		minSamples = 20
	}

	if w.adaptiveEWMA == nil {
		w.adaptiveEWMA = adaptive.NewEWMAMetrics(0.2, minSamples)
	}

	source := adaptive.NewEWMASignalSource(w.adaptiveEWMA, 0.2)

	var policy adaptive.Policy
	switch policyName {
	case "hybrid_aimd":
		p, err := adaptive.NewHybridAIMDPolicy(adaptive.PolicyConfig{
			MinC:             minC,
			MaxC:             maxC,
			TargetCPU:        float64(cfg.AdaptiveTargetCPU),
			TargetLatencyMs:  cfg.AdaptiveTargetLatencyMs,
			ErrorHi:          cfg.AdaptiveErrorHi,
			ErrorLo:          cfg.AdaptiveErrorLo,
			CPUHyst:          float64(cfg.AdaptiveHysteresis),
			AIStep:           cfg.AdaptiveAIStep,
			MDFactor:         cfg.AdaptiveMDFactor,
			CooldownUp:       cooldownUp,
			CooldownDown:     cooldownDown,
			HoldoffAfterDown: holdoffAfterDown,
			MinSamples:       minSamples,
		})
		if err != nil {
			w.logger.Warn().Err(err).Msg("adaptive concurrency: не вдалося ініціалізувати hybrid_aimd політику")
			return
		}
		policy = p
	default: // "cpu_only"
		target := cfg.AdaptiveTargetCPU
		if target <= 0 {
			target = 70
		}
		hyst := cfg.AdaptiveHysteresis
		if hyst <= 0 {
			hyst = 10
		}
		policy = adaptive.NewCPUOnlyPolicy(adaptive.CPUOnlyConfig{
			MinC:     minC,
			MaxC:     maxC,
			Target:   target,
			Hyst:     hyst,
			Cooldown: cooldownLegacy,
			MinHold:  cooldownLegacy,
		})
	}

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			sig, err := source.Signal(now)
			if err != nil {
				w.logger.Debug().Err(err).Msg("adaptive concurrency: помилка отримання сигналу")
				continue
			}

			if policyName == "cpu_only" && math.IsNaN(sig.CPU) {
				w.logger.Debug().Msg("adaptive concurrency: пропуск тіку (CPU недоступний для cpu-only)")
				continue
			}

			next, reason, changed := policy.Next(cur, sig, now)
			if !changed {
				continue
			}

			applied := w.executor.SetConcurrency(next)
			if applied == cur {
				continue
			}

			w.logger.Info().
				Int("concurrency_prev", cur).
				Int("concurrency_next", applied).
				Str("reason", reason).
				Float64("cpu", sig.CPU).
				Float64("latency_ewma_ms", sig.LatencyEWMA).
				Float64("error_ewma", sig.ErrorEWMA).
				Int("samples", sig.Samples).
				Msg("adaptive concurrency оновлено")

			cur = applied
		}
	}
}
