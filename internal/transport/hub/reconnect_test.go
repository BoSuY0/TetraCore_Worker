package hub

import (
	"testing"
	"time"
)

// TestDefaultReconnectConfig перевіряє, що DefaultReconnectConfig() повертає
// очікувані значення за замовчуванням.
func TestDefaultReconnectConfig(t *testing.T) {
	cfg := DefaultReconnectConfig()

	if cfg.InitialDelay != 5*time.Second {
		t.Errorf("InitialDelay: got %v, want %v", cfg.InitialDelay, 5*time.Second)
	}
	if cfg.MaxDelay != 300*time.Second {
		t.Errorf("MaxDelay: got %v, want %v", cfg.MaxDelay, 300*time.Second)
	}
	if cfg.Multiplier != 1.5 {
		t.Errorf("Multiplier: got %v, want %v", cfg.Multiplier, 1.5)
	}
	if cfg.CircuitBreakerMax != 5 {
		t.Errorf("CircuitBreakerMax: got %d, want %d", cfg.CircuitBreakerMax, 5)
	}
	if cfg.CircuitPause != 300*time.Second {
		t.Errorf("CircuitPause: got %v, want %v", cfg.CircuitPause, 300*time.Second)
	}
	if cfg.MaxFailures != 0 {
		t.Errorf("MaxFailures: got %d, want %d", cfg.MaxFailures, 0)
	}
	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval: got %v, want %v", cfg.CheckInterval, 30*time.Second)
	}
}

// TestReconnectConfigFromHubConfig перевіряє, що ReconnectConfigFromHubConfig
// коректно конвертує кастомні значення з HubConfig.
func TestReconnectConfigFromHubConfig(t *testing.T) {
	hubCfg := HubConfig{
		ReconnectDelay:    10 * time.Second,
		MaxReconnectDelay: 120 * time.Second,
	}

	rc := ReconnectConfigFromHubConfig(hubCfg, 0)

	if rc.InitialDelay != 10*time.Second {
		t.Errorf("InitialDelay: got %v, want %v", rc.InitialDelay, 10*time.Second)
	}
	if rc.MaxDelay != 120*time.Second {
		t.Errorf("MaxDelay: got %v, want %v", rc.MaxDelay, 120*time.Second)
	}

	// Решта полів мають залишитись за замовчуванням.
	if rc.Multiplier != 1.5 {
		t.Errorf("Multiplier: got %v, want %v", rc.Multiplier, 1.5)
	}
	if rc.CircuitBreakerMax != 5 {
		t.Errorf("CircuitBreakerMax: got %d, want %d", rc.CircuitBreakerMax, 5)
	}
	if rc.CircuitPause != 300*time.Second {
		t.Errorf("CircuitPause: got %v, want %v", rc.CircuitPause, 300*time.Second)
	}
	if rc.MaxFailures != 0 {
		t.Errorf("MaxFailures: got %d, want %d", rc.MaxFailures, 0)
	}
	if rc.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval: got %v, want %v", rc.CheckInterval, 30*time.Second)
	}
}

// TestReconnectConfigFromHubConfig_Defaults перевіряє, що при нульових
// значеннях затримок у HubConfig використовуються значення за замовчуванням.
func TestReconnectConfigFromHubConfig_Defaults(t *testing.T) {
	hubCfg := HubConfig{
		ReconnectDelay:    0,
		MaxReconnectDelay: 0,
	}

	rc := ReconnectConfigFromHubConfig(hubCfg, 0)

	defaults := DefaultReconnectConfig()

	if rc.InitialDelay != defaults.InitialDelay {
		t.Errorf("InitialDelay: got %v, want default %v", rc.InitialDelay, defaults.InitialDelay)
	}
	if rc.MaxDelay != defaults.MaxDelay {
		t.Errorf("MaxDelay: got %v, want default %v", rc.MaxDelay, defaults.MaxDelay)
	}
	if rc.Multiplier != defaults.Multiplier {
		t.Errorf("Multiplier: got %v, want default %v", rc.Multiplier, defaults.Multiplier)
	}
	if rc.CircuitBreakerMax != defaults.CircuitBreakerMax {
		t.Errorf("CircuitBreakerMax: got %d, want default %d", rc.CircuitBreakerMax, defaults.CircuitBreakerMax)
	}
	if rc.CircuitPause != defaults.CircuitPause {
		t.Errorf("CircuitPause: got %v, want default %v", rc.CircuitPause, defaults.CircuitPause)
	}
	if rc.MaxFailures != defaults.MaxFailures {
		t.Errorf("MaxFailures: got %d, want default %d", rc.MaxFailures, defaults.MaxFailures)
	}
	if rc.CheckInterval != defaults.CheckInterval {
		t.Errorf("CheckInterval: got %v, want default %v", rc.CheckInterval, defaults.CheckInterval)
	}
}

// TestReconnectConfigFromHubConfig_MaxFailures перевіряє, що параметр
// maxFailures коректно передається в ReconnectConfig.
func TestReconnectConfigFromHubConfig_MaxFailures(t *testing.T) {
	hubCfg := HubConfig{
		ReconnectDelay:    3 * time.Second,
		MaxReconnectDelay: 60 * time.Second,
	}

	rc := ReconnectConfigFromHubConfig(hubCfg, 10)

	if rc.MaxFailures != 10 {
		t.Errorf("MaxFailures: got %d, want %d", rc.MaxFailures, 10)
	}
	if rc.InitialDelay != 3*time.Second {
		t.Errorf("InitialDelay: got %v, want %v", rc.InitialDelay, 3*time.Second)
	}
	if rc.MaxDelay != 60*time.Second {
		t.Errorf("MaxDelay: got %v, want %v", rc.MaxDelay, 60*time.Second)
	}
}
