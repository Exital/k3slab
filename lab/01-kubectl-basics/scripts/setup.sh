#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

progress() {
  local pct="$1"
  shift
  if command -v k3slab-progress >/dev/null 2>&1; then
    k3slab-progress "${pct}" "$@"
  else
    printf '::k3slab-progress::%s::%s\n' "${pct}" "$*"
  fi
}

NGINX_IMAGE="${NGINX_IMAGE:-docker.io/library/nginx:1.27-alpine}"
BUSYBOX_IMAGE="${BUSYBOX_IMAGE:-docker.io/library/busybox:1.36}"

progress 10 "Pre-pulling images"
echo "[kubectl-basics] Pre-pulling images in parallel..."
pull_pids=()
for img in "${NGINX_IMAGE}" "${BUSYBOX_IMAGE}"; do
  k3s ctr images pull "${img}" &
  pull_pids+=("$!")
done
pull_ec=0
for pid in "${pull_pids[@]}"; do
  wait "${pid}" || pull_ec=1
done
if [[ "${pull_ec}" -ne 0 ]]; then
  echo "[kubectl-basics] One or more image pulls failed" >&2
  exit 1
fi

progress 50 "Applying lab manifests"
kubectl apply -f manifests/lab-env.yml

progress 70 "Waiting for deployments"
rollout_ec=0
kubectl rollout status deployment/web -n kubectl-basics --timeout=120s &
web_pid=$!
kubectl rollout status deployment/logger -n kubectl-basics --timeout=120s &
logger_pid=$!
wait "${web_pid}" || rollout_ec=1
wait "${logger_pid}" || rollout_ec=1
if [[ "${rollout_ec}" -ne 0 ]]; then
  echo "[kubectl-basics] Deployment rollout failed" >&2
  exit 1
fi

progress 95 "Finishing"
