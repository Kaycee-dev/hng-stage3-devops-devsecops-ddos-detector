# Evidence Log

Use this file during implementation and live validation.

## EV-001 - Local implementation baseline

- date: `2026-04-25`
- scope: repository scaffold for Stage 3 detector
- result: Go detector, Compose stack, Nginx JSON logging, docs, and test surface added
- next evidence:
  - `go test ./detector`
  - `docker compose config`
  - `docker pull kefaslungu/hng-nextcloud`
  - live VPS smoke test

## Screenshot Register

- [ ] `screenshots/Tool-running.png` - daemon running and processing logs
- [ ] `screenshots/Ban-slack.png` - Slack ban notification
- [ ] `screenshots/Unban-slack.png` - Slack unban notification
- [ ] `screenshots/Global-alert-slack.png` - Slack global anomaly notification
- [ ] `screenshots/Iptables-banned.png` - `sudo iptables -L -n` showing blocked IP
- [ ] `screenshots/Audit-log.png` - ban, unban, baseline recalculation lines
- [ ] `screenshots/Baseline-graph.png` - graph with at least two hourly slots
