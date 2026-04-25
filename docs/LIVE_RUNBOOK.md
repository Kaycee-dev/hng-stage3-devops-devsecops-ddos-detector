# Live Runbook

## Before The 12-Hour Window

1. Provision Ubuntu 22.04 or 24.04 with at least 2 vCPU and 2 GB RAM.
2. Point the DuckDNS hostname at the server IP.
3. Run `sudo bash scripts/bootstrap_vps.sh`.
4. Copy `.env.example` to `.env` and fill real values.
5. Add any trusted admin public IP/CIDR to `detector/config.yaml` allowlist.
6. Run `docker pull kefaslungu/hng-nextcloud:latest`.
7. Run `docker compose up -d --build`.
8. Run `bash scripts/smoke_test.sh`.

## During The Window

Every hour, record:

- `docker compose ps`
- `curl -fsS http://${DASHBOARD_HOST}/health`
- `docker compose logs detector --tail=50`
- `docker compose exec nginx tail -n 5 /var/log/nginx/hng-access.log`
- `sudo iptables -L -n`

## Evidence Capture

- Capture `Tool-running.png` after detector logs show parsed traffic or baseline recalculation.
- Capture `Ban-slack.png` immediately after `scripts/generate_attack_traffic.sh`.
- Capture `Iptables-banned.png` from `sudo iptables -L -n`.
- Capture `Unban-slack.png` after the scheduled unban.
- Capture `Global-alert-slack.png` after `scripts/generate_global_spike.sh`.
- Capture `Audit-log.png` from the detector audit log.
- Capture `Baseline-graph.png` from the dashboard after at least two hourly slots have different means.

## Final Submission

Submit only after README values are real and every screenshot exists.
