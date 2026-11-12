#!/bin/sh
# Runs the api and web microservices as one Podman pod (no compose provider
# needed). Manifest: deploy/podman/wheel-tuner.yaml.
#
#   scripts/podman.sh up [--pull]      build (or pull from quay.io) the images, start, wait for /healthz
#   scripts/podman.sh down [--volumes] stop the pod; --volumes also deletes the SQLite data
#   scripts/podman.sh logs [-f]        logs of both containers
#   scripts/podman.sh status
#
# Environment: PORT (host port, default 8080); QUAY_NAMESPACE and TAG pick the
# images for --pull (default bakhterets, latest).
set -eu

cd "$(dirname "$0")/.."

manifest=deploy/podman/wheel-tuner.yaml
pod=wheel-tuner
volume=wheel-tuner-data
port=${PORT:-8080}

usage() {
	echo "usage: $0 up [--pull] | down [--volumes] | logs [-f] | status" >&2
	exit 2
}

build_images() {
	podman build -t localhost/wheel-tuner-api:latest -f Dockerfile .
	podman build -t localhost/wheel-tuner-web:latest -f web/Dockerfile web
}

# The manifest references localhost/* images, so published ones are re-tagged.
pull_images() {
	src=quay.io/${QUAY_NAMESPACE:-bakhterets}
	for name in wheel-tuner-api wheel-tuner-web; do
		podman pull "$src/$name:${TAG:-latest}"
		podman tag "$src/$name:${TAG:-latest}" "localhost/$name:latest"
	done
}

wait_ready() {
	url=http://localhost:$port/healthz
	if ! command -v curl >/dev/null 2>&1; then
		echo "started; curl not found, open http://localhost:$port"
		return 0
	fi
	i=0
	until curl -fsS "$url" >/dev/null 2>&1; do
		i=$((i + 1))
		if [ "$i" -ge 30 ]; then
			echo "not ready after 60s: $url" >&2
			podman logs "$pod-api" >&2 || true
			podman logs "$pod-web" >&2 || true
			return 1
		fi
		sleep 2
	done
	echo "ready: http://localhost:$port"
}

cmd=${1:-up}
[ $# -gt 0 ] && shift

case $cmd in
up)
	pull=false
	for arg in "$@"; do
		case $arg in
		--pull) pull=true ;;
		*) usage ;;
		esac
	done
	if $pull; then pull_images; else build_images; fi
	if [ "$port" = 8080 ]; then
		podman kube play --replace "$manifest"
	else
		podman kube play --replace --publish "$port:8080" "$manifest"
	fi
	wait_ready
	;;
down)
	volumes=false
	for arg in "$@"; do
		case $arg in
		--volumes) volumes=true ;;
		*) usage ;;
		esac
	done
	if podman pod exists "$pod"; then
		podman kube down "$manifest"
	fi
	if $volumes && podman volume exists "$volume"; then
		podman volume rm "$volume"
	fi
	;;
logs)
	podman pod logs "$@" "$pod"
	;;
status)
	podman pod ps --filter "name=^$pod\$"
	podman ps -a --pod --filter "pod=$pod"
	;;
*)
	usage
	;;
esac
