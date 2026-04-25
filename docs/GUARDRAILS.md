# Guardrails

## Grader Traps

- Do not use Fail2Ban or any rate-limiting library.
- Do not replace or modify `kefaslungu/hng-nextcloud`.
- Do not fake the sliding window with per-minute counters.
- Do not hardcode effective mean; thresholds and floor values live in `detector/config.yaml`.
- Do not disable Nextcloud login or upload endpoints.
- Keep the dashboard reachable by domain or subdomain and Nextcloud reachable by IP only.
- Keep the stack live for the full announced 12-hour grading window.

## Blocking Safety

- The blocker refuses invalid IPs and configured allowlisted CIDRs.
- Private, loopback, link-local, and ULA ranges are allowlisted by default.
- Add the VPS admin IP/CIDR to `allowlist` before live testing if SSH originates from a fixed public IP.
- Confirm `iptables -L -n` before and after synthetic attacks.

## Evidence Discipline

- Capture screenshots with the exact filenames in `screenshots/`.
- Keep Slack messages visible when capturing ban, unban, and global alert evidence.
- Keep `docs/EVIDENCE_LOG.md` updated with commands, timestamps, and outcomes.
