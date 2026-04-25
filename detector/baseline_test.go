package main

import (
	"math"
	"testing"
	"time"
)

func TestTimeDequeRateEvictsOldEvents(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	var deque TimeDeque
	deque.PushBack(now.Add(-61 * time.Second))
	deque.PushBack(now.Add(-30 * time.Second))
	deque.PushBack(now)

	rate := deque.Rate(now, 60*time.Second)
	if deque.Len() != 2 {
		t.Fatalf("deque Len = %d, want 2", deque.Len())
	}
	if math.Abs(rate-(2.0/60.0)) > 0.0001 {
		t.Fatalf("rate = %f", rate)
	}
}

func TestBaselinePrefersCurrentHourWhenEnoughSamples(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Thresholds.MinHourlySamples = 2
	cfg.Thresholds.FloorMean = 0
	cfg.Thresholds.FloorStdDev = 0
	cfg.Thresholds.ErrorFloorMean = 0
	manager := NewBaselineManager(cfg)

	now := time.Date(2026, 4, 25, 12, 0, 3, 0, time.UTC)
	manager.Record(now.Add(-2*time.Second), false)
	manager.Record(now.Add(-1*time.Second), false)
	manager.Record(now, true)

	baseline := manager.Recalculate(now)
	if baseline.Source != "current-hour" {
		t.Fatalf("baseline Source = %q, want current-hour", baseline.Source)
	}
	if baseline.Samples < 2 {
		t.Fatalf("baseline Samples = %d", baseline.Samples)
	}
	if baseline.Mean <= 0 {
		t.Fatalf("baseline Mean = %f", baseline.Mean)
	}
	if baseline.ErrorMean <= 0 {
		t.Fatalf("baseline ErrorMean = %f", baseline.ErrorMean)
	}
}
