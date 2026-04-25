package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigParsesThresholdsAndLists(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/example")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`
log_path: /tmp/access.log
audit_log_path: /tmp/audit.log
dashboard_addr: :9090
slack_webhook_url: ${SLACK_WEBHOOK_URL}
startup_self_check: false
blocker_dry_run: true
tail_from_end: false
global_alert_cooldown_seconds: 10
ip_alert_cooldown_seconds: 5
thresholds:
  window_seconds: 30
  baseline_minutes: 15
  baseline_recalc_seconds: 20
  min_baseline_samples: 3
  min_hourly_samples: 2
  floor_mean: 0.2
  floor_stddev: 0.3
  error_floor_mean: 0.1
  zscore_threshold: 4
  multiplier_threshold: 6
  tightened_zscore_threshold: 2
  tightened_multiplier_threshold: 3
  error_surge_multiplier: 2
ban_durations:
  - 1s
  - permanent
allowlist:
  - 127.0.0.1/32
notifier:
  timeout_seconds: 2
dashboard:
  refresh_seconds: 2
  title: Test Detector
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.SlackWebhookURL != "https://hooks.slack.test/example" {
		t.Fatalf("SlackWebhookURL = %q", cfg.SlackWebhookURL)
	}
	if cfg.WindowDuration() != 30*time.Second {
		t.Fatalf("WindowDuration = %s", cfg.WindowDuration())
	}
	if len(cfg.BanDurations) != 2 || cfg.BanDurations[1].Label != "permanent" {
		t.Fatalf("BanDurations = %+v", cfg.BanDurations)
	}
	if len(cfg.Allowlist) != 1 {
		t.Fatalf("Allowlist = %+v", cfg.Allowlist)
	}
}
