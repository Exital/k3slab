#!/usr/bin/env bash
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export PATH="/usr/local/bin:/usr/local/sbin:/usr/bin:/sbin:/bin:${PATH}"

K9S_STAGED=/usr/local/lib/k3slab/k9s
: "${k9s_enable:=false}"
case "${k9s_enable,,}" in
  true|1|yes)
    echo "[k3slab] k9s_enable=true — enabling k9s"
    ln -sf "${K9S_STAGED}" /usr/local/bin/k9s
    export COLORTERM="${COLORTERM:-truecolor}"
    ;;
  *)
    rm -f /usr/local/bin/k9s 2>/dev/null || true
    ;;
esac

export K3SLAB_K3S_LOG_FILE="${K3SLAB_K3S_LOG_FILE:-/var/log/k3s-server.log}"
export K3SLAB_K3S_LOG_MAX_BYTES="${K3SLAB_K3S_LOG_MAX_BYTES:-10485760}" # 10 MiB
export K3SLAB_K3S_LOG_BACKUPS="${K3SLAB_K3S_LOG_BACKUPS:-3}"

# shellcheck source=/dev/null
source /usr/local/lib/k3slab/k3s-lifecycle.sh

rotate_k3s_log_if_needed() {
  [[ -f "${K3SLAB_K3S_LOG_FILE}" ]] || return 0
  local size
  size=$(wc -c < "${K3SLAB_K3S_LOG_FILE}" 2>/dev/null || echo 0)
  [[ "${size}" =~ ^[0-9]+$ ]] || return 0
  if (( size < K3SLAB_K3S_LOG_MAX_BYTES )); then
    return 0
  fi

  local backups="${K3SLAB_K3S_LOG_BACKUPS}"
  (( backups < 1 )) && backups=1
  local i
  for (( i=backups; i>=2; i-- )); do
    if [[ -f "${K3SLAB_K3S_LOG_FILE}.$((i-1))" ]]; then
      mv -f "${K3SLAB_K3S_LOG_FILE}.$((i-1))" "${K3SLAB_K3S_LOG_FILE}.${i}"
    fi
  done
  mv -f "${K3SLAB_K3S_LOG_FILE}" "${K3SLAB_K3S_LOG_FILE}.1"
  : > "${K3SLAB_K3S_LOG_FILE}"
}

start_k3s_log_rotator() {
  (
    while kill -0 "${K3S_PID}" 2>/dev/null; do
      rotate_k3s_log_if_needed
      sleep 15
    done
  ) &
  K3S_ROTATOR_PID=$!
}

echo "[k3slab] Starting K3s server..."
# Inside Docker, nested overlay often fails ("overlayfs snapshotter cannot be enabled...
# try fuse-overlayfs or native"). Native snapshotter is slower but reliable for local labs.
: "${K3SLAB_K3S_SNAPSHOTTER:=native}"
K3S_PID=$(start_k3s)
start_k3s_log_rotator

cleanup() {
  echo "[k3slab] Shutting down..."
  if [[ -n "${K3S_ROTATOR_PID:-}" ]] && kill -0 "${K3S_ROTATOR_PID}" 2>/dev/null; then
    kill "${K3S_ROTATOR_PID}" 2>/dev/null || true
    wait "${K3S_ROTATOR_PID}" 2>/dev/null || true
  fi
  if [[ -n "${APP_PID:-}" ]] && kill -0 "${APP_PID}" 2>/dev/null; then
    kill "${APP_PID}" 2>/dev/null || true
    wait "${APP_PID}" 2>/dev/null || true
  fi
  stop_k3s
}
trap cleanup SIGINT SIGTERM

wait_ready

export LABS_ROOT="${LABS_ROOT:-/lab}"
export LAB_ID="${LAB_ID:-01-k3s}"
export K3SLAB_LISTEN="${K3SLAB_LISTEN:-0.0.0.0:3010}"
export K3SLAB_STATIC_DIR="${K3SLAB_STATIC_DIR:-/app/frontend/dist}"

echo "[k3slab] Starting API on ${K3SLAB_LISTEN}..."
/app/k3slab &
APP_PID=$!

wait "${APP_PID}"
exit_code=$?
cleanup
exit "${exit_code}"
