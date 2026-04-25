package main

import (
	"testing"
)

func TestEvaluateFiresOnZScore(t *testing.T) {
	cfg := DefaultConfig()
	engine := &Engine{cfg: cfg}
	decision := engine.evaluate("198.51.100.23", "ip", 2.0, Baseline{Mean: 0.5, StdDev: 0.25}, false)
	if !decision.Fired {
		t.Fatal("expected decision to fire")
	}
	if decision.Condition == "" {
		t.Fatal("expected condition text")
	}
}

func TestEvaluateFiresOnMultiplier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Thresholds.ZScoreThreshold = 100
	engine := &Engine{cfg: cfg}
	decision := engine.evaluate("198.51.100.23", "ip", 3.1, Baseline{Mean: 0.5, StdDev: 1}, false)
	if !decision.Fired {
		t.Fatal("expected multiplier decision to fire")
	}
}

func TestBanDurationBackoffEndsPermanent(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.BanDurationFor(1); got.Label != "10m" || got.Permanent {
		t.Fatalf("first ban = %+v", got)
	}
	if got := cfg.BanDurationFor(3); got.Label != "2h" || got.Permanent {
		t.Fatalf("third ban = %+v", got)
	}
	if got := cfg.BanDurationFor(99); got.Label != "permanent" || !got.Permanent {
		t.Fatalf("later ban = %+v", got)
	}
}
