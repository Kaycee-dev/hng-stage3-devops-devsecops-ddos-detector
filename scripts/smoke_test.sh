#!/usr/bin/env bash
set -euo pipefail

SERVER_IP="${SERVER_IP:?set SERVER_IP}"
DASHBOARD_HOST="${DASHBOARD_HOST:?set DASHBOARD_HOST}"

echo "== Compose status =="
docker compose ps

echo "== Nextcloud by IP =="
curl --fail --silent --show-error "http://${SERVER_IP}/status.php" | head -c 300
echo

echo "== Dashboard by domain =="
curl --fail --silent --show-error "http://${DASHBOARD_HOST}/health"
echo

echo "== Nginx JSON log sample =="
docker compose exec nginx sh -c 'tail -n 3 /var/log/nginx/hng-access.log'

echo "== Detector metrics sample =="
curl --fail --silent --show-error "http://${DASHBOARD_HOST}/api/metrics" | head -c 1000
echo
