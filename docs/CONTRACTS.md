# Contracts

## Runtime Topology

- `nginx` is the only public HTTP entrypoint on port `80`.
- `nextcloud` uses the required image `kefaslungu/hng-nextcloud:latest` and is not published directly.
- `detector` runs continuously as a daemon, exposes the dashboard on host port `8081`, and uses `iptables` for host blocking.
- The named Docker volume is exactly `HNG-nginx-logs`.
- Nginx writes `/var/log/nginx/hng-access.log`; detector mounts the same volume read-only at `/var/log/nginx`.
- Nextcloud also mounts the log volume read-only to satisfy the brief.

## Log Contract

Every access log line is JSON and includes at least:

```json
{
  "source_ip": "198.51.100.23",
  "timestamp": "2026-04-25T12:00:00+00:00",
  "method": "GET",
  "path": "/index.php/login",
  "status": 200,
  "response_size": 1234
}
```

## Detection Contract

- Per-IP and global request windows use deque-backed 60-second windows.
- Baselines use rolling 30-minute per-second counts and recalculate every 60 seconds.
- Current-hour baseline is preferred once `min_hourly_samples` is reached.
- Detection fires when z-score exceeds `3.0` or rate exceeds `5x` baseline mean.
- Error surge tightens per-IP thresholds when 4xx/5xx rate exceeds `3x` baseline error rate.
- Per-IP anomaly blocks the IP and sends Slack within the response path.
- Global anomaly sends Slack only.

## Audit Contract

Audit lines follow:

```text
[timestamp] ACTION ip | condition | rate=... | baseline=mean=...,stddev=...,source=... | duration=...
```
