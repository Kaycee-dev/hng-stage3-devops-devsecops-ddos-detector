# HNG14 DevOps Stage 3 - DDoS Detection Engine

Go-based anomaly detection engine for the HNG Stage 3 Nextcloud brief. The stack runs the required `kefaslungu/hng-nextcloud` image behind Nginx, tails JSON access logs in real time, learns rolling baselines, blocks abusive IPs with `iptables`, sends Slack alerts, and serves a live metrics dashboard.

## Live Submission Values

Replace these before submission:

- Server IP: `SERVER_IP` from `.env`
- Metrics dashboard URL: `http://DASHBOARD_HOST`
- GitHub repo: `https://github.com/<your-user>/<your-public-repo>`
- Blog post: `https://<published-blog-url>`

A beginner-friendly draft is available at `docs/BLOG_POST_DRAFT.md` and should be published before final submission.

## Stack

| Component | Role |
| --- | --- |
| Nginx | Public reverse proxy, JSON access logs, real IP handling |
| Nextcloud | Required Docker image, reachable by server IP only |
| Detector | Go daemon for monitoring, baselines, blocking, Slack, dashboard |
| MariaDB | Persistent database for Nextcloud |
| Docker Compose | Runtime orchestration |

## Why Go

Go gives this project a small, reliable daemon with one static binary and no runtime package download step. The detector uses only the standard library, so the important logic is easy to inspect: deques, baseline calculations, anomaly decisions, `iptables`, Slack, and dashboard serving.

## Sliding Window

The detector keeps two 60-second deque-backed windows:

- one global request deque
- one per-IP request deque

Every parsed Nginx log line appends its timestamp to the relevant deque. Before calculating a rate, the detector evicts timestamps older than `now - 60s`. The request rate is:

```text
len(deque) / 60 seconds
```

Error traffic uses the same structure, but only stores 4xx/5xx requests.

## Baseline

Baselines are calculated from per-second request counts over the last 30 minutes. Every 60 seconds, the detector recalculates:

- mean requests per second
- standard deviation
- error mean
- error standard deviation

It also stores per-hour slots. Once the current hour has enough samples, that current-hour baseline becomes the effective baseline. Floor values in `detector/config.yaml` prevent division-by-zero and cold-start blind spots without hardcoding the effective mean.

## Detection Logic

For each request, the detector compares the current 60-second rate to the effective baseline:

```text
zscore = (current_rate - baseline_mean) / baseline_stddev
```

An anomaly fires when either condition is true:

- z-score exceeds `3.0`
- current rate is more than `5x` baseline mean

If an IP's 4xx/5xx rate reaches `3x` its baseline error rate, that IP gets tighter thresholds automatically.

## Blocking And Unban

Per-IP anomalies run:

```bash
iptables -I INPUT -s <ip> -j DROP
```

The blocker checks for existing rules first, refuses invalid IPs, and refuses configured allowlisted CIDRs. Global anomalies only send Slack alerts.

Ban backoff schedule:

1. 10 minutes
2. 30 minutes
3. 2 hours
4. permanent

Every ban, unban, and baseline recalculation writes an audit line in this format:

```text
[timestamp] ACTION ip | condition | rate=... | baseline=mean=...,stddev=...,source=... | duration=...
```

## Fresh VPS Setup

Use Ubuntu 22.04 or 24.04 with at least 2 vCPU and 2 GB RAM.

```bash
git clone https://github.com/<your-user>/<your-public-repo>.git
cd <your-public-repo>

sudo bash scripts/bootstrap_vps.sh
cp .env.example .env
nano .env
```

Set real values:

- `SERVER_IP`
- `DASHBOARD_HOST`
- strong DB and Nextcloud passwords
- `SLACK_WEBHOOK_URL`

Configure DuckDNS so `DASHBOARD_HOST` points to `SERVER_IP`, then start:

```bash
docker pull kefaslungu/hng-nextcloud:latest
docker compose up -d --build
docker compose ps
```

Verify:

```bash
export SERVER_IP=<your-server-ip>
export DASHBOARD_HOST=<your-duckdns-host>
bash scripts/smoke_test.sh
```

## Evidence Traffic

Per-IP ban test:

```bash
bash scripts/generate_attack_traffic.sh "http://${SERVER_IP}" 500
sudo iptables -L -n
docker compose logs detector --tail=100
```

Global alert test:

```bash
bash scripts/generate_global_spike.sh "http://${SERVER_IP}" 30 30
docker compose logs detector --tail=100
```

## Required Screenshots

Capture all files under `screenshots/`:

- `Tool-running.png`
- `Ban-slack.png`
- `Unban-slack.png`
- `Global-alert-slack.png`
- `Iptables-banned.png`
- `Audit-log.png`
- `Baseline-graph.png`

## Local Validation

```bash
go test ./detector
docker compose config
```

## Repo Layout

```text
detector/
  main.go
  monitor.go
  baseline.go
  detector.go
  blocker.go
  unbanner.go
  notifier.go
  dashboard.go
  config.yaml
nginx/
  nginx.conf
  hng.conf.template
docs/
  architecture.png
screenshots/
README.md
```
