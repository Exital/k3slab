#!/usr/bin/env bash
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export PATH="/usr/local/bin:/usr/local/sbin:/usr/bin:/sbin:/bin:${PATH}"

echo "[k3slab] Starting K3s server..."
# Inside Docker, nested overlay often fails ("overlayfs snapshotter cannot be enabled...
# try fuse-overlayfs or native"). Native snapshotter is slower but reliable for local labs.
: "${K3SLAB_K3S_SNAPSHOTTER:=native}"
k3s server \
  --write-kubeconfig-mode 644 \
  --bind-address 127.0.0.1 \
  --https-listen-port 6443 \
  --disable-network-policy \
  --disable=metrics-server \
  --snapshotter="${K3SLAB_K3S_SNAPSHOTTER}" \
  &

K3S_PID=$!

cleanup() {
  echo "[k3slab] Shutting down..."
  if [[ -n "${APP_PID:-}" ]] && kill -0 "${APP_PID}" 2>/dev/null; then
    kill "${APP_PID}" 2>/dev/null || true
    wait "${APP_PID}" 2>/dev/null || true
  fi
  if kill -0 "${K3S_PID}" 2>/dev/null; then
    kill "${K3S_PID}" 2>/dev/null || true
    wait "${K3S_PID}" 2>/dev/null || true
  fi
}
trap cleanup SIGINT SIGTERM

echo "[k3slab] Waiting for cluster to become Ready..."
start_ts=$(date +%s)
while true; do
  if kubectl get nodes 2>/dev/null | grep -q '\sReady\s'; then
    break
  fi
  now_ts=$(date +%s)
  if (( now_ts - start_ts > 300 )); then
    echo "[k3slab] ERROR: K3s did not become ready in time."
    exit 1
  fi
  sleep 2
done

echo "[k3slab] Cluster is Ready."

export LAB_ROOT="${LAB_ROOT:-/lab/k3s}"
export K3SLAB_LISTEN="${K3SLAB_LISTEN:-0.0.0.0:3010}"
export K3SLAB_STATIC_DIR="${K3SLAB_STATIC_DIR:-/app/frontend/dist}"

echo "[k3slab] Starting API on ${K3SLAB_LISTEN}..."
/app/k3slab &
APP_PID=$!

wait "${APP_PID}"
exit_code=$?
cleanup
exit "${exit_code}"
