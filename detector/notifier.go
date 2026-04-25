package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Alert struct {
	Action      string
	IP          string
	Condition   string
	Rate        float64
	Baseline    Baseline
	BanDuration string
	Timestamp   time.Time
}

type Notifier struct {
	webhookURL string
	client     *http.Client
}

func NewNotifier(cfg Config) *Notifier {
	return &Notifier{
		webhookURL: strings.TrimSpace(cfg.SlackWebhookURL),
		client:     &http.Client{Timeout: cfg.Notifier.Timeout},
	}
}

func (n *Notifier) Send(ctx context.Context, alert Alert) error {
	if n.webhookURL == "" {
		return nil
	}
	text := n.format(alert)
	payload := map[string]interface{}{
		"text": text,
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": text,
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) format(alert Alert) string {
	ip := alert.IP
	if ip == "" {
		ip = "global"
	}
	duration := alert.BanDuration
	if duration == "" {
		duration = "n/a"
	}
	return fmt.Sprintf(
		"*%s* ip=%s condition=%s current_rate=%.2f req/s baseline_mean=%.2f baseline_stddev=%.2f timestamp=%s ban_duration=%s",
		alert.Action,
		ip,
		alert.Condition,
		alert.Rate,
		alert.Baseline.Mean,
		alert.Baseline.StdDev,
		alert.Timestamp.UTC().Format(time.RFC3339),
		duration,
	)
}
