# Decisions Log

## DEC-001 - Go implementation

- decision: implement the detector in Go
- reason: single static binary, strong standard library, no third-party rate-limiting package risk

## DEC-002 - No runtime Go dependencies

- decision: use the Go standard library only
- reason: avoids dependency downloads on a fresh VPS and keeps the implementation inspectable

## DEC-003 - Host-network detector

- decision: detector uses `network_mode: host` plus `NET_ADMIN` and `NET_RAW`
- reason: `iptables` must affect the host network namespace, not a private container namespace

## DEC-004 - DuckDNS dashboard

- decision: dashboard is routed through a DuckDNS host while Nextcloud remains on the public IP default vhost
- reason: matches the grading requirement that the submitted dashboard is served at a domain or subdomain
