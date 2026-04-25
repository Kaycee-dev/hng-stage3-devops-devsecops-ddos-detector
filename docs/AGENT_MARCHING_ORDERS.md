# Agent Marching Orders - Stage 3 DDoS Detector

Use this as the first message for a new implementation agent.

## Mission

You are taking over `c:\Users\Hp\Documents\hng14\devops-stage3`. The goal is a 100/100 HNG14 DevOps Stage 3 submission: a Go-based anomaly/DDoS detector deployed beside the required `kefaslungu/hng-nextcloud` image, with Nginx JSON logs, deque sliding windows, rolling baselines, Slack alerts, `iptables` blocking, auto-unban, dashboard, screenshots, README, and blog evidence.

Do not replace or modify the required Nextcloud image. Do not use Fail2Ban or rate-limiting libraries. Keep all detector thresholds/configuration in `detector/config.yaml`.

## Current Repo State

The repo has already been scaffolded and partially implemented:

- Go module: `go.mod`
- Detector daemon: `detector/*.go`
- Detector config: `detector/config.yaml`
- Detector Dockerfile: `detector/Dockerfile`
- Compose stack: `docker-compose.yml`
- Nginx config: `nginx/nginx.conf`, `nginx/hng.conf.template`
- Governance docs: `docs/CONTRACTS.md`, `docs/GUARDRAILS.md`, `docs/TEST_MATRIX.md`, `docs/EVIDENCE_LOG.md`, `docs/SUBMISSION_CHECKLIST.md`, `docs/LIVE_RUNBOOK.md`
- Blog draft: `docs/BLOG_POST_DRAFT.md`
- Architecture diagram: `docs/architecture.png`
- Screenshot register: `screenshots/README.md`
- CI workflow: `.github/workflows/ci.yml`
- VPS/test scripts: `scripts/*.sh`

Local validation already completed:

- `docker compose --env-file .env.example config --quiet` passes.
- Code scan found no disallowed detector libraries or unfinished TODO/FIXME markers.

Known local validation gaps:

- Go is not installed on this Windows machine, so `gofmt` and `go test ./detector` were not run locally.
- Docker CLI exists, but Docker Desktop service was stopped and could not be started from this session, so Docker image build was not run locally.

## First Actions

1. Inspect the repo before editing:

```powershell
git status --short
Get-ChildItem -Force
Get-ChildItem -Recurse -File detector,docs,nginx,scripts,.github | Select-Object FullName,Length
```

2. If Go is available, immediately run:

```powershell
gofmt -w detector/*.go
go test ./detector
```

3. If Docker is available, run:

```powershell
docker compose --env-file .env.example config --quiet
docker build -f detector/Dockerfile .
```

4. Fix any compile, formatting, or test failures before touching feature scope.

## Implementation Priorities

### 1. Detector Correctness

Verify these behaviors in code and tests:

- Nginx log parser accepts JSON with `source_ip`, `timestamp`, `method`, `path`, `status`, `response_size`.
- Log monitor tails continuously and survives missing/rotated log files.
- Global and per-IP request windows use deque eviction over the last 60 seconds.
- Baselines use per-second counts over a 30-minute rolling window.
- Baselines recalculate every 60 seconds.
- Current-hour baseline is preferred once enough samples exist.
- Detection fires when z-score exceeds `3.0` or current rate exceeds `5x` baseline mean.
- Error surge tightens per-IP thresholds when 4xx/5xx rate exceeds `3x` baseline error mean.
- Per-IP anomaly blocks and alerts; global anomaly only alerts.
- Ban schedule is 10 minutes, 30 minutes, 2 hours, then permanent.
- Audit log writes every ban, unban, and baseline recalculation in the required format.

### 2. Deployment Correctness

Verify and harden:

- Compose volume is named exactly `HNG-nginx-logs`.
- Nginx writes `/var/log/nginx/hng-access.log`.
- Detector mounts Nginx logs read-only.
- Nextcloud mounts Nginx logs read-only.
- Nextcloud is not directly host-published.
- Nginx default server routes IP traffic to Nextcloud.
- DuckDNS host routes dashboard traffic to detector.
- Detector runs with enough privileges for host `iptables` blocking.
- Nginx forwards real client IP using `X-Forwarded-For`.

### 3. Evidence And Submission

Before final submission, every checkbox in `docs/SUBMISSION_CHECKLIST.md` must be complete. Required screenshots must exist under `screenshots/` with exact filenames:

- `Tool-running.png`
- `Ban-slack.png`
- `Unban-slack.png`
- `Global-alert-slack.png`
- `Iptables-banned.png`
- `Audit-log.png`
- `Baseline-graph.png`

README must contain real final values:

- server IP
- dashboard URL
- public GitHub repo link
- published beginner-friendly blog link

## VPS Execution Path

On the fresh Ubuntu VPS:

```bash
git clone https://github.com/<your-user>/<your-public-repo>.git
cd <your-public-repo>
sudo bash scripts/bootstrap_vps.sh
cp .env.example .env
nano .env
docker pull kefaslungu/hng-nextcloud:latest
docker compose up -d --build
docker compose ps
export SERVER_IP=<server-ip>
export DASHBOARD_HOST=<duckdns-host>
bash scripts/smoke_test.sh
```

Synthetic evidence traffic:

```bash
bash scripts/generate_attack_traffic.sh "http://${SERVER_IP}" 500
sudo iptables -L -n
bash scripts/generate_global_spike.sh "http://${SERVER_IP}" 30 30
docker compose logs detector --tail=200
```

## Scoring Gates

Treat the assignment as these scoring gates:

- 25 points: VPS, Compose, Nginx JSON logs, named volume, real-IP forwarding, Nextcloud/IP-only topology.
- 30 points: Go detector correctness, deque windows, rolling baseline, hourly baseline, anomaly rules, error surge handling.
- 15 points: Slack alerts, `iptables` blocking, unban backoff, allowlist safety.
- 15 points: dashboard, audit log, screenshots, baseline graph.
- 10 points: README, setup instructions, public repo, beginner-friendly blog.
- 5 points: resilience polish, health checks, smoke scripts, clean evidence workflow.

## Non-Negotiables

- Do not use Fail2Ban.
- Do not use slowapi, `golang.org/x/time/rate`, or any rate-limiting package.
- Do not replace `kefaslungu/hng-nextcloud`.
- Do not hardcode effective mean or detection thresholds in logic.
- Do not fake the sliding window with per-minute counters.
- Do not disable Nextcloud login or upload endpoints.
- Do not submit until the server is live for the required 12-hour grading window.

## Final Agent Output Expected

When complete, report:

- files changed
- tests/checks run and exact results
- live server IP and dashboard URL
- screenshot completion status
- blog URL
- any remaining risk before submission
