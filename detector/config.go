package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ThresholdConfig struct {
	WindowSeconds                int
	BaselineMinutes              int
	BaselineRecalcSeconds        int
	MinBaselineSamples           int
	MinHourlySamples             int
	FloorMean                    float64
	FloorStdDev                  float64
	ErrorFloorMean               float64
	ZScoreThreshold              float64
	MultiplierThreshold          float64
	TightenedZScoreThreshold     float64
	TightenedMultiplierThreshold float64
	ErrorSurgeMultiplier         float64
}

type BanDuration struct {
	Label     string
	Duration  time.Duration
	Permanent bool
}

type NotifierConfig struct {
	Timeout time.Duration
}

type DashboardConfig struct {
	RefreshSeconds int
	Title          string
}

type Config struct {
	LogPath             string
	AuditLogPath        string
	DashboardAddr       string
	SlackWebhookURL     string
	StartupSelfCheck    bool
	BlockerDryRun       bool
	TailFromEnd         bool
	GlobalAlertCooldown time.Duration
	IPAlertCooldown     time.Duration
	Thresholds          ThresholdConfig
	BanDurations        []BanDuration
	Allowlist           []string
	Notifier            NotifierConfig
	Dashboard           DashboardConfig
}

func DefaultConfig() Config {
	return Config{
		LogPath:             "/var/log/nginx/hng-access.log",
		AuditLogPath:        "/var/log/detector/audit.log",
		DashboardAddr:       ":8081",
		StartupSelfCheck:    true,
		BlockerDryRun:       false,
		TailFromEnd:         true,
		GlobalAlertCooldown: 60 * time.Second,
		IPAlertCooldown:     30 * time.Second,
		Thresholds: ThresholdConfig{
			WindowSeconds:                60,
			BaselineMinutes:              30,
			BaselineRecalcSeconds:        60,
			MinBaselineSamples:           120,
			MinHourlySamples:             120,
			FloorMean:                    0.10,
			FloorStdDev:                  0.10,
			ErrorFloorMean:               0.02,
			ZScoreThreshold:              3.0,
			MultiplierThreshold:          5.0,
			TightenedZScoreThreshold:     2.0,
			TightenedMultiplierThreshold: 3.0,
			ErrorSurgeMultiplier:         3.0,
		},
		BanDurations: []BanDuration{
			{Label: "10m", Duration: 10 * time.Minute},
			{Label: "30m", Duration: 30 * time.Minute},
			{Label: "2h", Duration: 2 * time.Hour},
			{Label: "permanent", Permanent: true},
		},
		Allowlist: []string{
			"127.0.0.0/8",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"169.254.0.0/16",
			"::1/128",
			"fc00::/7",
			"fe80::/10",
		},
		Notifier: NotifierConfig{Timeout: 5 * time.Second},
		Dashboard: DashboardConfig{
			RefreshSeconds: 3,
			Title:          "HNG DDoS Detector",
		},
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		raw := stripComment(scanner.Text())
		if strings.TrimSpace(raw) == "" {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		line := strings.TrimSpace(raw)
		if indent == 0 && strings.HasSuffix(line, ":") {
			section = strings.TrimSpace(strings.TrimSuffix(line, ":"))
			continue
		}

		if strings.HasPrefix(line, "- ") {
			if err := assignConfigListValue(&cfg, section, normalizeValue(strings.TrimSpace(strings.TrimPrefix(line, "- ")))); err != nil {
				return cfg, fmt.Errorf("config line %d: %w", lineNo, err)
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return cfg, fmt.Errorf("config line %d: expected key: value", lineNo)
		}
		key = strings.TrimSpace(key)
		value = normalizeValue(value)
		if indent == 0 {
			section = ""
		}
		if err := assignConfigValue(&cfg, section, key, value); err != nil {
			return cfg, fmt.Errorf("config line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	if len(cfg.BanDurations) == 0 {
		return cfg, fmt.Errorf("at least one ban duration is required")
	}
	if cfg.Thresholds.WindowSeconds <= 0 || cfg.Thresholds.BaselineMinutes <= 0 {
		return cfg, fmt.Errorf("window_seconds and baseline_minutes must be positive")
	}
	if cfg.Dashboard.RefreshSeconds <= 0 || cfg.Dashboard.RefreshSeconds > 3 {
		cfg.Dashboard.RefreshSeconds = 3
	}
	return cfg, nil
}

func stripComment(line string) string {
	for i, r := range line {
		if r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return strings.TrimRight(line[:i], " \t")
		}
	}
	return line
}

func normalizeValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return os.ExpandEnv(value)
}

func assignConfigListValue(cfg *Config, section, value string) error {
	switch section {
	case "ban_durations":
		duration, err := parseBanDuration(value)
		if err != nil {
			return err
		}
		if len(cfg.BanDurations) == 4 &&
			cfg.BanDurations[0].Label == "10m" &&
			cfg.BanDurations[1].Label == "30m" &&
			cfg.BanDurations[2].Label == "2h" &&
			cfg.BanDurations[3].Label == "permanent" {
			cfg.BanDurations = nil
		}
		cfg.BanDurations = append(cfg.BanDurations, duration)
	case "allowlist":
		if len(cfg.Allowlist) == 8 && cfg.Allowlist[0] == "127.0.0.0/8" {
			cfg.Allowlist = nil
		}
		cfg.Allowlist = append(cfg.Allowlist, value)
	default:
		return fmt.Errorf("unknown list section %q", section)
	}
	return nil
}

func assignConfigValue(cfg *Config, section, key, value string) error {
	switch section {
	case "":
		return assignTopLevelConfigValue(cfg, key, value)
	case "thresholds":
		return assignThresholdConfigValue(&cfg.Thresholds, key, value)
	case "notifier":
		if key == "timeout_seconds" {
			seconds, err := parseInt(value)
			if err != nil {
				return err
			}
			cfg.Notifier.Timeout = time.Duration(seconds) * time.Second
			return nil
		}
	case "dashboard":
		switch key {
		case "refresh_seconds":
			seconds, err := parseInt(value)
			if err != nil {
				return err
			}
			cfg.Dashboard.RefreshSeconds = seconds
			return nil
		case "title":
			cfg.Dashboard.Title = value
			return nil
		}
	}
	return fmt.Errorf("unknown config key %q in section %q", key, section)
}

func assignTopLevelConfigValue(cfg *Config, key, value string) error {
	switch key {
	case "log_path":
		cfg.LogPath = value
	case "audit_log_path":
		cfg.AuditLogPath = value
	case "dashboard_addr":
		cfg.DashboardAddr = value
	case "slack_webhook_url":
		cfg.SlackWebhookURL = value
	case "startup_self_check":
		parsed, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.StartupSelfCheck = parsed
	case "blocker_dry_run":
		parsed, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.BlockerDryRun = parsed
	case "tail_from_end":
		parsed, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.TailFromEnd = parsed
	case "global_alert_cooldown_seconds":
		seconds, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.GlobalAlertCooldown = time.Duration(seconds) * time.Second
	case "ip_alert_cooldown_seconds":
		seconds, err := parseInt(value)
		if err != nil {
			return err
		}
		cfg.IPAlertCooldown = time.Duration(seconds) * time.Second
	default:
		return fmt.Errorf("unknown top-level config key %q", key)
	}
	return nil
}

func assignThresholdConfigValue(cfg *ThresholdConfig, key, value string) error {
	switch key {
	case "window_seconds":
		return setInt(value, &cfg.WindowSeconds)
	case "baseline_minutes":
		return setInt(value, &cfg.BaselineMinutes)
	case "baseline_recalc_seconds":
		return setInt(value, &cfg.BaselineRecalcSeconds)
	case "min_baseline_samples":
		return setInt(value, &cfg.MinBaselineSamples)
	case "min_hourly_samples":
		return setInt(value, &cfg.MinHourlySamples)
	case "floor_mean":
		return setFloat(value, &cfg.FloorMean)
	case "floor_stddev":
		return setFloat(value, &cfg.FloorStdDev)
	case "error_floor_mean":
		return setFloat(value, &cfg.ErrorFloorMean)
	case "zscore_threshold":
		return setFloat(value, &cfg.ZScoreThreshold)
	case "multiplier_threshold":
		return setFloat(value, &cfg.MultiplierThreshold)
	case "tightened_zscore_threshold":
		return setFloat(value, &cfg.TightenedZScoreThreshold)
	case "tightened_multiplier_threshold":
		return setFloat(value, &cfg.TightenedMultiplierThreshold)
	case "error_surge_multiplier":
		return setFloat(value, &cfg.ErrorSurgeMultiplier)
	default:
		return fmt.Errorf("unknown threshold config key %q", key)
	}
}

func parseBanDuration(value string) (BanDuration, error) {
	if strings.EqualFold(value, "permanent") {
		return BanDuration{Label: "permanent", Permanent: true}, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return BanDuration{}, fmt.Errorf("invalid ban duration %q: %w", value, err)
	}
	return BanDuration{Label: value, Duration: duration}, nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "yes", "1", "on":
		return true, nil
	case "false", "no", "0", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", value)
	}
}

func parseInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid int %q: %w", value, err)
	}
	return parsed, nil
}

func setInt(value string, target *int) error {
	parsed, err := parseInt(value)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}

func setFloat(value string, target *float64) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("invalid float %q: %w", value, err)
	}
	*target = parsed
	return nil
}

func (cfg Config) WindowDuration() time.Duration {
	return time.Duration(cfg.Thresholds.WindowSeconds) * time.Second
}

func (cfg Config) BaselineDuration() time.Duration {
	return time.Duration(cfg.Thresholds.BaselineMinutes) * time.Minute
}

func (cfg Config) RecalcDuration() time.Duration {
	return time.Duration(cfg.Thresholds.BaselineRecalcSeconds) * time.Second
}

func (cfg Config) BanDurationFor(count int) BanDuration {
	if count <= 0 {
		count = 1
	}
	if count > len(cfg.BanDurations) {
		return cfg.BanDurations[len(cfg.BanDurations)-1]
	}
	return cfg.BanDurations[count-1]
}
