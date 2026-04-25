# Test Matrix

| Area | Check | Evidence |
| --- | --- | --- |
| Parser | Valid JSON log line parses into source IP, timestamp, method, path, status, size | `go test ./detector` |
| Parser | Malformed or incomplete line is skipped without crashing | `go test ./detector` |
| Window | Deque evicts events older than 60 seconds | `go test ./detector` |
| Baseline | Rolling baseline calculates mean/stddev from per-second counts | `go test ./detector` |
| Baseline | Current-hour baseline wins after enough samples | `go test ./detector` |
| Detection | z-score and multiplier thresholds both fire | `go test ./detector` |
| Blocking | Synthetic per-IP spike creates `iptables DROP` rule | `screenshots/Iptables-banned.png` |
| Slack | Ban, unban, and global anomaly notifications include rate and baseline | Slack screenshots |
| Audit | Ban, unban, and baseline recalculation lines use required format | `screenshots/Audit-log.png` |
| Dashboard | Metrics refresh at <= 3 seconds and show required fields | dashboard screenshot |
| Live | Nextcloud works by IP, dashboard works by DuckDNS, stack survives 12 hours | evidence log |
