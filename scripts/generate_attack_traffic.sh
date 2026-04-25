#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:?usage: scripts/generate_attack_traffic.sh http://SERVER_IP [requests]}"
REQUESTS="${2:-500}"

for i in $(seq 1 "${REQUESTS}"); do
  curl --silent --output /dev/null --header "X-Forwarded-For: 198.51.100.23" "${TARGET}/index.php/login?attack=${i}" &
  if (( i % 50 == 0 )); then
    wait
  fi
done
wait

echo "sent ${REQUESTS} requests with X-Forwarded-For=198.51.100.23"
