#!/usr/bin/env bash
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export PATH="/go/bin:/usr/local/bin:/usr/local/sbin:/usr/bin:/sbin:/bin:${PATH}"
export LABS_ROOT="${LABS_ROOT:-/workspace/lab}"
export LAB_ID="${LAB_ID:-01-kubectl-basics}"
export K3SLAB_LISTEN="${K3SLAB_LISTEN:-0.0.0.0:3010}"

# shellcheck source=/dev/null
source /usr/local/lib/k3slab/k3s-lifecycle.sh

echo "[k3slab-dev] Preparing Go module dependencies..."
(cd /workspace/app/backend && go mod tidy)

echo "[k3slab-dev] Applying cluster profile for lab: ${LAB_ID}"
(cd /workspace/app/backend && go run . apply-cluster-profile "${LABS_ROOT}" "${LAB_ID}")

echo "[k3slab-dev] Starting K3s server..."
: "${K3SLAB_K3S_SNAPSHOTTER:=native}"
K3S_PID=$(start_k3s)

cleanup() {
  echo "[k3slab-dev] Shutting down..."
  if [[ -n "${AIR_PID:-}" ]] && kill -0 "${AIR_PID}" 2>/dev/null; then
    kill "${AIR_PID}" 2>/dev/null || true
    wait "${AIR_PID}" 2>/dev/null || true
  fi
  if [[ -n "${READY_WAIT_PID:-}" ]] && kill -0 "${READY_WAIT_PID}" 2>/dev/null; then
    kill "${READY_WAIT_PID}" 2>/dev/null || true
    wait "${READY_WAIT_PID}" 2>/dev/null || true
  fi
  stop_k3s
}
trap cleanup SIGINT SIGTERM

echo "[k3slab-dev] Waiting for cluster in background..."
(
  if ! wait_ready; then
    echo "[k3slab-dev] Cluster still unavailable; backend remains up."
  fi
) &
READY_WAIT_PID=$!

echo "[k3slab-dev] Starting backend watcher (air)..."
(cd /workspace/app/backend && air -c .air.toml) &
AIR_PID=$!

wait "${AIR_PID}"
exit_code=$?
cleanup
exit "${exit_code}"
