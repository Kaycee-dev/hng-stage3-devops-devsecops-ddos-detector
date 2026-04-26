package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type IPStats struct {
	Requests  TimeDeque
	Errors    TimeDeque
	Baseline  *BaselineManager
	LastSeen  time.Time
	Tightened bool
	Total     int64
}

type BanRecord struct {
	IP          string    `json:"ip"`
	Condition   string    `json:"condition"`
	Rate        float64   `json:"rate"`
	Baseline    Baseline  `json:"baseline"`
	Duration    string    `json:"duration"`
	BannedAt    time.Time `json:"banned_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	Permanent   bool      `json:"permanent"`
	Offense     int       `json:"offense"`
	RulePresent bool      `json:"rule_present"`
}

type DetectionDecision struct {
	Fired     bool
	IP        string
	Scope     string
	Condition string
	Rate      float64
	Baseline  Baseline
	ZScore    float64
}

type Auditor struct {
	mu   sync.Mutex
	file *os.File
}

func NewAuditor(path string) (*Auditor, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Auditor{file: file}, nil
}

func (a *Auditor) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.file.Close()
}

func (a *Auditor) Write(action, ip, condition string, rate float64, baseline Baseline, duration string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	line := fmt.Sprintf(
		"[%s] %s %s | %s | rate=%.2f | baseline=mean=%.2f,stddev=%.2f,source=%s | duration=%s\n",
		time.Now().UTC().Format(time.RFC3339),
		action,
		ip,
		condition,
		rate,
		baseline.Mean,
		baseline.StdDev,
		baseline.Source,
		duration,
	)
	_, _ = a.file.WriteString(line)
	_ = a.file.Sync()
	log.Print(line)
}

type Engine struct {
	mu              sync.RWMutex
	cfg             Config
	blocker         *Blocker
	notifier        *Notifier
	auditor         *Auditor
	globalRequests  TimeDeque
	globalErrors    TimeDeque
	globalBaseline  *BaselineManager
	ips             map[string]*IPStats
	bans            map[string]*BanRecord
	banHistory      map[string]int
	startedAt       time.Time
	lastGlobalAlert time.Time
}

func NewEngine(cfg Config, blocker *Blocker, notifier *Notifier, auditor *Auditor) *Engine {
	return &Engine{
		cfg:            cfg,
		blocker:        blocker,
		notifier:       notifier,
		auditor:        auditor,
		globalBaseline: NewBaselineManager(cfg),
		ips:            make(map[string]*IPStats),
		bans:           make(map[string]*BanRecord),
		banHistory:     make(map[string]int),
		startedAt:      time.Now(),
	}
}

func (e *Engine) Run(ctx context.Context) error {
	events := make(chan AccessLog, 4096)
	monitor := LogMonitor{
		Path:        e.cfg.LogPath,
		TailFromEnd: e.cfg.TailFromEnd,
		Logger:      log.Default(),
	}
	go func() {
		if err := monitor.Tail(ctx, events); err != nil && ctx.Err() == nil {
			log.Printf("log monitor stopped: %v", err)
		}
	}()

	recalcTicker := time.NewTicker(e.cfg.RecalcDuration())
	defer recalcTicker.Stop()
	e.RecalculateBaselines(time.Now())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-events:
			e.Process(event)
		case now := <-recalcTicker.C:
			e.RecalculateBaselines(now)
		}
	}
}

// Process is called for every parsed access log line. It updates the
// global and per-IP sliding windows, records into the corresponding
// baselines, evaluates both the global and per-IP detection rules, and
// dispatches blocks or alerts when those rules fire. Per-IP anomalies
// trigger an iptables DROP plus a Slack alert; global anomalies only
// alert (the brief forbids blocking everyone during a global spike).
func (e *Engine) Process(entry AccessLog) {
	now := entry.ParsedTime
	if now.IsZero() {
		now = time.Now()
	}
	// Defend against clock-skewed log lines from the future.
	if now.After(time.Now().Add(5 * time.Minute)) {
		now = time.Now()
	}
	isError := entry.Status >= 400

	var ipDecision, globalDecision DetectionDecision
	e.mu.Lock()
	// Update the global 60s deque and global baseline counters.
	e.globalRequests.PushBack(now)
	e.globalErrors.EvictBefore(now.Add(-e.cfg.WindowDuration()))
	if isError {
		e.globalErrors.PushBack(now)
	}
	e.globalBaseline.Record(now, isError)
	globalRate := e.globalRequests.Rate(now, e.cfg.WindowDuration())
	globalDecision = e.evaluate("", "global", globalRate, e.globalBaseline.Current(), false)

	// Update the per-IP 60s deque and per-IP baseline counters.
	stats := e.ipStats(entry.SourceIP)
	stats.Requests.PushBack(now)
	if isError {
		stats.Errors.PushBack(now)
	}
	stats.LastSeen = now
	stats.Total++
	stats.Baseline.Record(now, isError)
	ipRate := stats.Requests.Rate(now, e.cfg.WindowDuration())
	errorRate := stats.Errors.Rate(now, e.cfg.WindowDuration())
	ipBaseline := stats.Baseline.Current()
	// Error-surge rule: when the IP's error rate exceeds N x its baseline
	// error mean, future evaluations for this IP use tightened thresholds.
	stats.Tightened = errorRate > 0 && errorRate >= e.cfg.Thresholds.ErrorSurgeMultiplier*ipBaseline.ErrorMean
	// Allowlisted IPs are never evaluated for blocks; the blocker also
	// refuses them defensively if a decision were ever to leak through.
	if !e.blocker.IsAllowedIP(entry.SourceIP) {
		ipDecision = e.evaluate(entry.SourceIP, "ip", ipRate, ipBaseline, stats.Tightened)
	}
	e.mu.Unlock()

	if ipDecision.Fired {
		e.handleIPAnomaly(ipDecision)
	}
	if globalDecision.Fired {
		e.handleGlobalAnomaly(globalDecision)
	}
}

func (e *Engine) ipStats(ip string) *IPStats {
	stats := e.ips[ip]
	if stats == nil {
		stats = &IPStats{Baseline: NewBaselineManager(e.cfg)}
		e.ips[ip] = stats
	}
	return stats
}

// evaluate applies the two anomaly rules from the brief in order:
//  1. z-score = (rate - baseline.mean) / baseline.stddev > Z threshold
//  2. rate > multiplier x baseline.mean
//
// Whichever fires first marks the decision. If the IP has been flagged
// by the error-surge rule, the tightened thresholds (lower Z, lower
// multiplier) are used instead. Thresholds come from config; nothing in
// this function is hardcoded.
func (e *Engine) evaluate(ip, scope string, rate float64, baseline Baseline, tightened bool) DetectionDecision {
	zThreshold := e.cfg.Thresholds.ZScoreThreshold
	multiplier := e.cfg.Thresholds.MultiplierThreshold
	if tightened {
		zThreshold = e.cfg.Thresholds.TightenedZScoreThreshold
		multiplier = e.cfg.Thresholds.TightenedMultiplierThreshold
	}
	zScore := 0.0
	if baseline.StdDev > 0 {
		zScore = (rate - baseline.Mean) / baseline.StdDev
	}
	if zScore > zThreshold {
		return DetectionDecision{
			Fired:     true,
			IP:        ip,
			Scope:     scope,
			Condition: fmt.Sprintf("zscore %.2f > %.2f", zScore, zThreshold),
			Rate:      rate,
			Baseline:  baseline,
			ZScore:    zScore,
		}
	}
	if rate > baseline.Mean*multiplier {
		return DetectionDecision{
			Fired:     true,
			IP:        ip,
			Scope:     scope,
			Condition: fmt.Sprintf("rate %.2f > %.2fx baseline mean", rate, multiplier),
			Rate:      rate,
			Baseline:  baseline,
			ZScore:    zScore,
		}
	}
	return DetectionDecision{Scope: scope, IP: ip, Rate: rate, Baseline: baseline, ZScore: zScore}
}

// handleIPAnomaly records a ban entry, installs the iptables DROP rule,
// writes the audit line, sends a Slack alert, and schedules the unban
// according to the backoff ladder (10m, 30m, 2h, permanent). If the
// iptables call fails the in-memory ban is rolled back so the IP can be
// retried; the failure is recorded as BLOCK_FAILED in the audit log.
func (e *Engine) handleIPAnomaly(decision DetectionDecision) {
	if e.blocker.IsAllowedIP(decision.IP) {
		return
	}

	e.mu.Lock()
	if _, exists := e.bans[decision.IP]; exists {
		e.mu.Unlock()
		return
	}
	e.banHistory[decision.IP]++
	offense := e.banHistory[decision.IP]
	banDuration := e.cfg.BanDurationFor(offense)
	record := &BanRecord{
		IP:        decision.IP,
		Condition: decision.Condition,
		Rate:      decision.Rate,
		Baseline:  decision.Baseline,
		Duration:  banDuration.Label,
		BannedAt:  time.Now(),
		Permanent: banDuration.Permanent,
		Offense:   offense,
	}
	if !banDuration.Permanent {
		record.ExpiresAt = record.BannedAt.Add(banDuration.Duration)
	}
	e.bans[decision.IP] = record
	e.mu.Unlock()

	if err := e.blocker.Block(decision.IP); err != nil {
		e.mu.Lock()
		delete(e.bans, decision.IP)
		e.mu.Unlock()
		e.auditor.Write("BLOCK_FAILED", decision.IP, err.Error(), decision.Rate, decision.Baseline, banDuration.Label)
		return
	}

	e.mu.Lock()
	record.RulePresent = true
	e.mu.Unlock()
	e.auditor.Write("BAN", decision.IP, decision.Condition, decision.Rate, decision.Baseline, banDuration.Label)
	go e.sendAlert(context.Background(), Alert{
		Action:      "BAN",
		IP:          decision.IP,
		Condition:   decision.Condition,
		Rate:        decision.Rate,
		Baseline:    decision.Baseline,
		BanDuration: banDuration.Label,
		Timestamp:   time.Now(),
	})
	if !banDuration.Permanent {
		ScheduleUnban(context.Background(), banDuration.Duration, func() {
			e.Unban(decision.IP, "scheduled backoff release")
		})
	}
}

// handleGlobalAnomaly sends a Slack alert and writes a GLOBAL_ALERT audit
// line. It deliberately does not block any IPs: under a global spike,
// blocking specific IPs would hide the underlying cause and blocking
// everyone would deny service to legitimate users. The cooldown
// suppresses repeated alerts during a sustained event.
func (e *Engine) handleGlobalAnomaly(decision DetectionDecision) {
	e.mu.Lock()
	if time.Since(e.lastGlobalAlert) < e.cfg.GlobalAlertCooldown {
		e.mu.Unlock()
		return
	}
	e.lastGlobalAlert = time.Now()
	e.mu.Unlock()

	e.auditor.Write("GLOBAL_ALERT", "global", decision.Condition, decision.Rate, decision.Baseline, "none")
	go e.sendAlert(context.Background(), Alert{
		Action:    "GLOBAL_ALERT",
		Condition: decision.Condition,
		Rate:      decision.Rate,
		Baseline:  decision.Baseline,
		Timestamp: time.Now(),
	})
}

func (e *Engine) Unban(ip, condition string) {
	e.mu.Lock()
	record, exists := e.bans[ip]
	if !exists || record.Permanent {
		e.mu.Unlock()
		return
	}
	delete(e.bans, ip)
	e.mu.Unlock()

	baseline := record.Baseline
	if err := e.blocker.Unblock(ip); err != nil {
		e.auditor.Write("UNBAN_FAILED", ip, err.Error(), record.Rate, baseline, record.Duration)
		return
	}
	e.auditor.Write("UNBAN", ip, condition, record.Rate, baseline, record.Duration)
	go e.sendAlert(context.Background(), Alert{
		Action:      "UNBAN",
		IP:          ip,
		Condition:   condition,
		Rate:        record.Rate,
		Baseline:    baseline,
		BanDuration: record.Duration,
		Timestamp:   time.Now(),
	})
}

func (e *Engine) sendAlert(ctx context.Context, alert Alert) {
	if err := e.notifier.Send(ctx, alert); err != nil {
		e.auditor.Write("SLACK_FAILED", firstNonEmpty(alert.IP, "global"), err.Error(), alert.Rate, alert.Baseline, alert.BanDuration)
	}
}

// RecalculateBaselines runs every BaselineRecalcSeconds. It refreshes
// the global baseline and every per-IP baseline, garbage-collects IP
// stats that have been silent for two hours and are not currently
// banned, and writes a BASELINE audit entry so the operator has a
// time-stamped record that the detector is alive and learning.
func (e *Engine) RecalculateBaselines(now time.Time) {
	e.mu.Lock()
	global := e.globalBaseline.Recalculate(now)
	for ip, stats := range e.ips {
		stats.Baseline.Recalculate(now)
		if stats.Requests.Rate(now, e.cfg.WindowDuration()) == 0 && now.Sub(stats.LastSeen) > 2*time.Hour {
			if _, banned := e.bans[ip]; !banned {
				delete(e.ips, ip)
			}
		}
	}
	e.mu.Unlock()
	e.auditor.Write("BASELINE", "global", "recalculated", 0, global, e.cfg.BaselineDuration().String())
}

type TopIP struct {
	IP       string  `json:"ip"`
	Rate     float64 `json:"rate"`
	Requests int     `json:"requests"`
}

type MetricsSnapshot struct {
	UptimeSeconds      int64           `json:"uptime_seconds"`
	GlobalReqPerSec    float64         `json:"global_req_per_sec"`
	TopIPs             []TopIP         `json:"top_ips"`
	BannedIPs          []*BanRecord    `json:"banned_ips"`
	CPUPercent         float64         `json:"cpu_percent"`
	MemoryPercent      float64         `json:"memory_percent"`
	EffectiveMean      float64         `json:"effective_mean"`
	EffectiveStdDev    float64         `json:"effective_stddev"`
	BaselineSource     string          `json:"baseline_source"`
	LastBaselineAt     string          `json:"last_baseline_at"`
	BaselineHistory    []BaselinePoint `json:"baseline_history"`
	DashboardRefresh   int             `json:"dashboard_refresh_seconds"`
	AuditLogPath       string          `json:"audit_log_path"`
	TrackedSourceCount int             `json:"tracked_source_count"`
}

func (e *Engine) Snapshot(cpu, memory float64) MetricsSnapshot {
	now := time.Now()
	e.mu.Lock()
	globalRate := e.globalRequests.Rate(now, e.cfg.WindowDuration())
	baseline := e.globalBaseline.Current()
	history := e.globalBaseline.History()
	top := make([]TopIP, 0, len(e.ips))
	for ip, stats := range e.ips {
		rate := stats.Requests.Rate(now, e.cfg.WindowDuration())
		if rate > 0 {
			top = append(top, TopIP{IP: ip, Rate: rate, Requests: stats.Requests.Len()})
		}
	}
	sort.Slice(top, func(i, j int) bool {
		return top[i].Rate > top[j].Rate
	})
	if len(top) > 10 {
		top = top[:10]
	}
	bans := make([]*BanRecord, 0, len(e.bans))
	for _, ban := range e.bans {
		copy := *ban
		bans = append(bans, &copy)
	}
	sort.Slice(bans, func(i, j int) bool {
		return bans[i].BannedAt.After(bans[j].BannedAt)
	})
	sourceCount := len(e.ips)
	e.mu.Unlock()

	lastBaselineAt := ""
	if !baseline.RecalculatedAt.IsZero() {
		lastBaselineAt = baseline.RecalculatedAt.UTC().Format(time.RFC3339)
	}
	return MetricsSnapshot{
		UptimeSeconds:      int64(time.Since(e.startedAt).Seconds()),
		GlobalReqPerSec:    globalRate,
		TopIPs:             top,
		BannedIPs:          bans,
		CPUPercent:         cpu,
		MemoryPercent:      memory,
		EffectiveMean:      baseline.Mean,
		EffectiveStdDev:    baseline.StdDev,
		BaselineSource:     baseline.Source,
		LastBaselineAt:     lastBaselineAt,
		BaselineHistory:    history,
		DashboardRefresh:   e.cfg.Dashboard.RefreshSeconds,
		AuditLogPath:       e.cfg.AuditLogPath,
		TrackedSourceCount: sourceCount,
	}
}

func (e *Engine) DumpSnapshot() string {
	snapshot := e.Snapshot(0, 0)
	body, _ := json.MarshalIndent(snapshot, "", "  ")
	return string(body)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
