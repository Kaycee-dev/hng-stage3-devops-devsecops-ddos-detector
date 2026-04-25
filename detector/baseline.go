package main

import (
	"math"
	"time"
)

type TimeDeque struct {
	values []time.Time
	head   int
}

func (d *TimeDeque) PushBack(value time.Time) {
	d.values = append(d.values, value)
}

func (d *TimeDeque) EvictBefore(cutoff time.Time) {
	for d.head < len(d.values) && d.values[d.head].Before(cutoff) {
		d.head++
	}
	if d.head > 1024 && d.head*2 > len(d.values) {
		copy(d.values, d.values[d.head:])
		d.values = d.values[:len(d.values)-d.head]
		d.head = 0
	}
}

func (d *TimeDeque) Len() int {
	return len(d.values) - d.head
}

func (d *TimeDeque) Rate(now time.Time, window time.Duration) float64 {
	d.EvictBefore(now.Add(-window))
	if window <= 0 {
		return 0
	}
	return float64(d.Len()) / window.Seconds()
}

type Baseline struct {
	Mean           float64   `json:"mean"`
	StdDev         float64   `json:"stddev"`
	ErrorMean      float64   `json:"error_mean"`
	ErrorStdDev    float64   `json:"error_stddev"`
	Samples        int       `json:"samples"`
	Source         string    `json:"source"`
	RecalculatedAt time.Time `json:"recalculated_at"`
}

type BaselinePoint struct {
	Timestamp  time.Time `json:"timestamp"`
	Mean       float64   `json:"mean"`
	StdDev     float64   `json:"stddev"`
	ErrorMean  float64   `json:"error_mean"`
	Samples    int       `json:"samples"`
	Source     string    `json:"source"`
	WindowName string    `json:"window_name"`
}

type secondSlot struct {
	Counts      map[int64]int
	ErrorCounts map[int64]int
	FirstSeen   time.Time
}

type BaselineManager struct {
	window           time.Duration
	minSamples       int
	minHourlySamples int
	floorMean        float64
	floorStdDev      float64
	errorFloorMean   float64
	rollingCounts    map[int64]int
	rollingErrors    map[int64]int
	hourly           map[int64]*secondSlot
	firstSeen        time.Time
	current          Baseline
	history          []BaselinePoint
}

func NewBaselineManager(cfg Config) *BaselineManager {
	return &BaselineManager{
		window:           cfg.BaselineDuration(),
		minSamples:       cfg.Thresholds.MinBaselineSamples,
		minHourlySamples: cfg.Thresholds.MinHourlySamples,
		floorMean:        cfg.Thresholds.FloorMean,
		floorStdDev:      cfg.Thresholds.FloorStdDev,
		errorFloorMean:   cfg.Thresholds.ErrorFloorMean,
		rollingCounts:    make(map[int64]int),
		rollingErrors:    make(map[int64]int),
		hourly:           make(map[int64]*secondSlot),
	}
}

func (b *BaselineManager) Record(at time.Time, isError bool) {
	sec := at.Unix()
	if b.firstSeen.IsZero() || at.Before(b.firstSeen) {
		b.firstSeen = at
	}
	b.rollingCounts[sec]++
	if isError {
		b.rollingErrors[sec]++
	}

	hour := at.Truncate(time.Hour).Unix()
	slot := b.hourly[hour]
	if slot == nil {
		slot = &secondSlot{
			Counts:      make(map[int64]int),
			ErrorCounts: make(map[int64]int),
			FirstSeen:   at,
		}
		b.hourly[hour] = slot
	}
	if at.Before(slot.FirstSeen) {
		slot.FirstSeen = at
	}
	slot.Counts[sec]++
	if isError {
		slot.ErrorCounts[sec]++
	}
}

func (b *BaselineManager) Recalculate(now time.Time) Baseline {
	b.cleanup(now)
	rolling := b.computeRolling(now)
	currentHour := b.computeCurrentHour(now)
	chosen := rolling
	if currentHour.Samples >= b.minHourlySamples {
		chosen = currentHour
		chosen.Source = "current-hour"
	} else {
		chosen.Source = "rolling-30m"
	}
	chosen.Mean = maxFloat(chosen.Mean, b.floorMean)
	chosen.StdDev = maxFloat(chosen.StdDev, b.floorStdDev)
	chosen.ErrorMean = maxFloat(chosen.ErrorMean, b.errorFloorMean)
	chosen.RecalculatedAt = now
	b.current = chosen
	b.history = append(b.history, BaselinePoint{
		Timestamp:  now,
		Mean:       chosen.Mean,
		StdDev:     chosen.StdDev,
		ErrorMean:  chosen.ErrorMean,
		Samples:    chosen.Samples,
		Source:     chosen.Source,
		WindowName: now.Truncate(time.Hour).Format("15:00"),
	})
	if len(b.history) > 720 {
		b.history = append([]BaselinePoint(nil), b.history[len(b.history)-720:]...)
	}
	return chosen
}

func (b *BaselineManager) Current() Baseline {
	if !b.current.RecalculatedAt.IsZero() {
		return b.current
	}
	return Baseline{
		Mean:      b.floorMean,
		StdDev:    b.floorStdDev,
		ErrorMean: b.errorFloorMean,
		Source:    "floor",
	}
}

func (b *BaselineManager) History() []BaselinePoint {
	return append([]BaselinePoint(nil), b.history...)
}

func (b *BaselineManager) computeRolling(now time.Time) Baseline {
	if b.firstSeen.IsZero() {
		return Baseline{Samples: 0}
	}
	start := now.Add(-b.window).Add(time.Second)
	if b.firstSeen.After(start) {
		start = b.firstSeen
	}
	return computeBaseline(start, now, b.rollingCounts, b.rollingErrors)
}

func (b *BaselineManager) computeCurrentHour(now time.Time) Baseline {
	hourStart := now.Truncate(time.Hour)
	slot := b.hourly[hourStart.Unix()]
	if slot == nil {
		return Baseline{Samples: 0}
	}
	start := hourStart
	if slot.FirstSeen.After(start) {
		start = slot.FirstSeen
	}
	return computeBaseline(start, now, slot.Counts, slot.ErrorCounts)
}

func computeBaseline(start, end time.Time, counts, errors map[int64]int) Baseline {
	if end.Before(start) {
		return Baseline{}
	}
	startSec := start.Unix()
	endSec := end.Unix()
	samples := int(endSec-startSec) + 1
	if samples <= 0 {
		return Baseline{}
	}

	var sum, errorSum float64
	for sec := startSec; sec <= endSec; sec++ {
		sum += float64(counts[sec])
		errorSum += float64(errors[sec])
	}
	mean := sum / float64(samples)
	errorMean := errorSum / float64(samples)

	var variance, errorVariance float64
	for sec := startSec; sec <= endSec; sec++ {
		diff := float64(counts[sec]) - mean
		variance += diff * diff
		errorDiff := float64(errors[sec]) - errorMean
		errorVariance += errorDiff * errorDiff
	}
	return Baseline{
		Mean:        mean,
		StdDev:      math.Sqrt(variance / float64(samples)),
		ErrorMean:   errorMean,
		ErrorStdDev: math.Sqrt(errorVariance / float64(samples)),
		Samples:     samples,
	}
}

func (b *BaselineManager) cleanup(now time.Time) {
	cutoff := now.Add(-b.window).Unix()
	for sec := range b.rollingCounts {
		if sec < cutoff {
			delete(b.rollingCounts, sec)
		}
	}
	for sec := range b.rollingErrors {
		if sec < cutoff {
			delete(b.rollingErrors, sec)
		}
	}
	hourCutoff := now.Add(-24 * time.Hour).Truncate(time.Hour).Unix()
	for hour := range b.hourly {
		if hour < hourCutoff {
			delete(b.hourly, hour)
		}
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
