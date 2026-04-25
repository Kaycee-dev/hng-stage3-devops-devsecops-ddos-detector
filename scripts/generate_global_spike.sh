#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:?usage: scripts/generate_global_spike.sh http://SERVER_IP [sources] [requests_per_source]}"
SOURCES="${2:-30}"
PER_SOURCE="${3:-30}"

for i in $(seq 1 "${SOURCES}"); do
  ip="203.0.113.${i}"
  for j in $(seq 1 "${PER_SOURCE}"); do
    curl --silent --output /dev/null --header "X-Forwarded-For: ${ip}" "${TARGET}/remote.php/dav/files/hng-${i}-${j}" &
  done
done
wait

echo "sent $(( SOURCES * PER_SOURCE )) distributed requests"
