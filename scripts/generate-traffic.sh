#!/bin/sh
set -eu
# Небольшой Go-клиент уже собран в demo-tools; Go/jq на host не нужны.
docker compose run --rm --no-deps demo-tools traffic "${TRAFFIC_COUNT:-30}" "${TRAFFIC_INTERVAL:-2s}"
