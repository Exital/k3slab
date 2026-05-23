#!/usr/bin/env bash
# Shared K3s start/stop/wait/reset helpers for entrypoint and cluster reset API.

: "${KUBECONFIG:=/etc/rancher/k3s/k3s.yaml}"
: "${K3SLAB_PID_FILE:=/run/k3slab/k3s.pid}"
: "${K3S_DATA_DIR:=/var/lib/rancher/k3s}"
: "${K3SLAB_K3S_LOG_FILE:=/var/log/k3s-server.log}"
: "${K3SLAB_K3S_SNAPSHOTTER:=native}"
: "${K3SLAB_K3S_READY_TIMEOUT:=300}"

k3slab_k3s_server_args() {
  echo \
    --write-kubeconfig-mode 644 \
    --bind-address 0.0.0.0 \
    --https-listen-port 6443 \
    --disable-network-policy \
    --disable=metrics-server \
    --snapshotter="${K3SLAB_K3S_SNAPSHOTTER}"
}

start_k3s() {
  mkdir -p "$(dirname "${K3SLAB_K3S_LOG_FILE}")" "$(dirname "${K3SLAB_PID_FILE}")"
  : > "${K3SLAB_K3S_LOG_FILE}"

  # shellcheck disable=SC2046
  k3s server $(k3slab_k3s_server_args) >> "${K3SLAB_K3S_LOG_FILE}" 2>&1 &
  local pid=$!
  echo "${pid}" > "${K3SLAB_PID_FILE}"
  echo "${pid}"
}

k3slab_k3s_processes_running() {
  pgrep -f '[k]3s server' >/dev/null 2>&1 \
    || pgrep -f '[k]3s/agent/containerd' >/dev/null 2>&1 \
    || pgrep -f 'containerd-shim' >/dev/null 2>&1
}

killall_k3s() {
  local pid=""
  if [[ -f "${K3SLAB_PID_FILE}" ]]; then
    pid=$(tr -d '[:space:]' < "${K3SLAB_PID_FILE}" || true)
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill -TERM "${pid}" 2>/dev/null || true
    fi
  fi

  if ! k3s killall 2>/dev/null; then
    pkill -TERM -f '[k]3s server' 2>/dev/null || true
    sleep 2
    pkill -KILL -f '[k]3s server' 2>/dev/null || true
    pkill -KILL -f 'containerd-shim' 2>/dev/null || true
    pkill -KILL -f '[k]3s/agent/containerd' 2>/dev/null || true
  fi

  local waited=0
  while k3slab_k3s_processes_running && (( waited < 45 )); do
    sleep 1
    waited=$((waited + 1))
  done

  rm -f "${K3SLAB_PID_FILE}"
}

stop_k3s() {
  killall_k3s
}

unmount_k3s_tmpmounts() {
  local tmp="${K3S_DATA_DIR}/agent/containerd/tmpmounts"
  [[ -d "${tmp}" ]] || return 0
  local m
  for m in "${tmp}"/containerd-mount*; do
    [[ -d "${m}" ]] || continue
    umount -l "${m}" 2>/dev/null || true
  done
}

wipe_k3s_data() {
  local attempt
  for attempt in $(seq 1 10); do
    unmount_k3s_tmpmounts
    if rm -rf "${K3S_DATA_DIR}" 2>/dev/null && [[ ! -e "${K3S_DATA_DIR}" ]]; then
      return 0
    fi
    echo "[k3slab] wipe attempt ${attempt}/10 failed, retrying..."
    killall_k3s
    sleep 2
  done
  echo "[k3slab] ERROR: could not remove ${K3S_DATA_DIR}"
  return 1
}

restore_k3s_after_reset_failure() {
  echo "[k3slab] Trying to restore K3s after failed reset..."
  start_k3s >/dev/null || return 1
  wait_ready
}

wait_ready() {
  echo "[k3slab] Waiting for cluster to become Ready..."
  local start_ts now_ts
  start_ts=$(date +%s)
  while true; do
    if kubectl get nodes 2>/dev/null | grep -q '\sReady\s'; then
      echo "[k3slab] Cluster is Ready."
      return 0
    fi
    now_ts=$(date +%s)
    if (( now_ts - start_ts > K3SLAB_K3S_READY_TIMEOUT )); then
      echo "[k3slab] ERROR: K3s did not become ready in time."
      echo "[k3slab] Last K3s log lines (${K3SLAB_K3S_LOG_FILE}):"
      tail -n 80 "${K3SLAB_K3S_LOG_FILE}" 2>/dev/null || true
      return 1
    fi
    sleep 2
  done
}

reset_cluster() {
  echo "[k3slab] Resetting cluster..."
  stop_k3s
  if ! wipe_k3s_data; then
    restore_k3s_after_reset_failure || true
    return 1
  fi
  start_k3s >/dev/null
  wait_ready
}
