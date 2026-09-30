package adaptive

import (
	"math"
	"testing"
	"time"
)

func TestHybridAIMD_Congestion_Decrease_AndCooldown(t *testing.T) {
	p, err := NewHybridAIMDPolicy(PolicyConfig{
		MinC:            1,
		MaxC:            32,
		TargetLatencyMs: 1000,
		AIStep:          1,
		MDFactor:        0.5,
		CooldownDown:    10 * time.Second,
		MinSamples:      1,
	})
	if err != nil {
		t.Fatalf("NewHybridAIMDPolicy() помилка: %v", err)
	}

	now := time.Unix(0, 0)
	cur := 10

	next, reason, changed := p.Next(cur, AdaptiveSignal{
		CPU:         10,
		LatencyEWMA: 2000, // > 1000*1.2
		ErrorEWMA:   0,
		Samples:     10,
	}, now)
	if !changed || next >= cur || reason != "latency" {
		t.Fatalf("очікували decrease по latency: next=%d reason=%q changed=%v", next, reason, changed)
	}

	cur = next
	next2, reason2, changed2 := p.Next(cur, AdaptiveSignal{
		CPU:         10,
		LatencyEWMA: 2000,
		ErrorEWMA:   0,
		Samples:     10,
	}, now.Add(1*time.Second))
	if changed2 || next2 != cur || reason2 != "cooldown" {
		t.Fatalf("очікували cooldown після decrease: next=%d reason=%q changed=%v", next2, reason2, changed2)
	}
}

func TestHybridAIMD_Underutilized_Increase_AndCooldown(t *testing.T) {
	p, err := NewHybridAIMDPolicy(PolicyConfig{
		MinC:            1,
		MaxC:            32,
		TargetCPU:       70,
		CPUHyst:         10,
		TargetLatencyMs: 1000,
		ErrorLo:         0.1,
		AIStep:          1,
		MDFactor:        0.7,
		CooldownUp:      10 * time.Second,
		MinSamples:      1,
	})
	if err != nil {
		t.Fatalf("NewHybridAIMDPolicy() помилка: %v", err)
	}

	now := time.Unix(0, 0)
	cur := 5

	next, reason, changed := p.Next(cur, AdaptiveSignal{
		CPU:         40,  // < 70-10
		LatencyEWMA: 500, // < 1000
		ErrorEWMA:   0,
		Samples:     10,
	}, now)
	if !changed || next != cur+1 || reason != "low_load" {
		t.Fatalf("очікували increase: next=%d reason=%q changed=%v", next, reason, changed)
	}

	cur = next
	next2, reason2, changed2 := p.Next(cur, AdaptiveSignal{
		CPU:         40,
		LatencyEWMA: 500,
		ErrorEWMA:   0,
		Samples:     10,
	}, now.Add(1*time.Second))
	if changed2 || next2 != cur || reason2 != "cooldown" {
		t.Fatalf("очікували cooldown після increase: next=%d reason=%q changed=%v", next2, reason2, changed2)
	}
}

func TestHybridAIMD_Holdoff_BlocksIncrease_AfterDecrease(t *testing.T) {
	p, err := NewHybridAIMDPolicy(PolicyConfig{
		MinC:             1,
		MaxC:             32,
		TargetLatencyMs:  1000,
		ErrorHi:          0.2,
		ErrorLo:          0.05,
		AIStep:           1,
		MDFactor:         0.5,
		HoldoffAfterDown: 30 * time.Second,
		MinSamples:       1,
	})
	if err != nil {
		t.Fatalf("NewHybridAIMDPolicy() помилка: %v", err)
	}

	now := time.Unix(0, 0)
	cur := 10

	next, reason, changed := p.Next(cur, AdaptiveSignal{
		CPU:         10,
		LatencyEWMA: 2000,
		ErrorEWMA:   0,
		Samples:     10,
	}, now)
	if !changed || next >= cur || reason != "latency" {
		t.Fatalf("очікували decrease перед holdoff: next=%d reason=%q changed=%v", next, reason, changed)
	}

	cur = next
	next2, reason2, changed2 := p.Next(cur, AdaptiveSignal{
		CPU:         10,
		LatencyEWMA: 500,
		ErrorEWMA:   0,
		Samples:     10,
	}, now.Add(1*time.Second))
	if changed2 || next2 != cur || reason2 != "holdoff" {
		t.Fatalf("очікували holdoff: next=%d reason=%q changed=%v", next2, reason2, changed2)
	}
}

func TestHybridAIMD_MinSamples_Blocks(t *testing.T) {
	p, err := NewHybridAIMDPolicy(PolicyConfig{
		MinC:       1,
		MaxC:       32,
		AIStep:     1,
		MDFactor:   0.7,
		MinSamples: 3,
	})
	if err != nil {
		t.Fatalf("NewHybridAIMDPolicy() помилка: %v", err)
	}

	next, reason, changed := p.Next(5, AdaptiveSignal{
		CPU:     10,
		Samples: 2,
	}, time.Unix(0, 0))
	if changed || next != 5 || reason != "min_samples" {
		t.Fatalf("очікували min_samples: next=%d reason=%q changed=%v", next, reason, changed)
	}
}

func TestHybridAIMD_CPUNaN_DecisionByLatencyOrError(t *testing.T) {
	p, err := NewHybridAIMDPolicy(PolicyConfig{
		MinC:            1,
		MaxC:            32,
		TargetLatencyMs: 1000,
		ErrorHi:         0.2,
		AIStep:          1,
		MDFactor:        0.5,
		MinSamples:      1,
	})
	if err != nil {
		t.Fatalf("NewHybridAIMDPolicy() помилка: %v", err)
	}

	cur := 10
	next, reason, changed := p.Next(cur, AdaptiveSignal{
		CPU:         math.NaN(),
		LatencyEWMA: 2000,
		ErrorEWMA:   0,
		Samples:     10,
	}, time.Unix(0, 0))
	if !changed || next >= cur || reason != "latency" {
		t.Fatalf("очікували decrease по latency без CPU: next=%d reason=%q changed=%v", next, reason, changed)
	}
}

func TestHybridAIMD_ClampMinMax(t *testing.T) {
	p, err := NewHybridAIMDPolicy(PolicyConfig{
		MinC:            2,
		MaxC:            4,
		TargetCPU:       70,
		CPUHyst:         10,
		TargetLatencyMs: 1000,
		ErrorLo:         0.1,
		ErrorHi:         0.2,
		AIStep:          10,  // спеціально завеликий крок, щоб перевірити clamp
		MDFactor:        0.1, // агресивний decrease
		MinSamples:      1,
	})
	if err != nil {
		t.Fatalf("NewHybridAIMDPolicy() помилка: %v", err)
	}

	// Increase не має виходити за MaxC.
	cur := 4
	next, _, changed := p.Next(cur, AdaptiveSignal{
		CPU:         0,
		LatencyEWMA: 10,
		ErrorEWMA:   0,
		Samples:     10,
	}, time.Unix(0, 0))
	if changed || next != 4 {
		t.Fatalf("очікували clamp до MaxC без зміни: next=%d changed=%v", next, changed)
	}

	// Decrease не має падати нижче MinC.
	cur = 2
	next2, _, changed2 := p.Next(cur, AdaptiveSignal{
		CPU:         0,
		LatencyEWMA: 2000,
		ErrorEWMA:   0,
		Samples:     10,
	}, time.Unix(0, 0))
	if changed2 || next2 != 2 {
		t.Fatalf("очікували clamp до MinC без зміни: next=%d changed=%v", next2, changed2)
	}
}
