#!/bin/sh
# Probes a running stack through the web proxy: frontend, proxy, backend, DB.
# Usage: scripts/smoke.sh [base-url]   (default http://localhost:8080)
set -eu

base=${1:-http://localhost:8080}

i=0
until curl -fsS "$base/healthz" >/dev/null 2>&1; do
	i=$((i + 1))
	if [ "$i" -ge 30 ]; then
		echo "not ready after 60s: $base/healthz" >&2
		exit 1
	fi
	sleep 2
done

curl -fsS "$base/livez"
curl -fsS "$base/healthz"
curl -fsS "$base/" | grep -q 'Wheel Tuner'
curl -fsS "$base/api/tensiometers" | grep -q 'ztto-tc-02'
curl -fsS "$base/api/components/hubs" | grep -q 'Fade SP'
echo "smoke ok: $base"
