#!/usr/bin/env bash
# Shared K3s start/stop/wait/reset helpers for entrypoint and cluster reset API.

: "${KUBECONFIG:=/etc/rancher/k3s/k3s.yaml}"
: "${K3SLAB_PID_FILE:=/run/k3slab/k3s.pid}"
: "${K3S_DATA_DIR:=/var/lib/rancher/k3s}"
: "${K3SLAB_K3S_LOG_FILE:=/var/log/k3s-server.log}"
: "${K3SLAB_K3S_SNAPSHOTTER:=native}"
: "${K3SLAB_K3S_READY_TIMEOUT:=300}"
: "${K3SLAB_CLUSTER_PROFILE:=/run/k3slab/cluster-profile.env}"

k3slab_load_cluster_profile() {
  K3SLAB_DISABLE_TRAEFIK=false
  if [[ -f "${K3SLAB_CLUSTER_PROFILE}" ]]; then
    # shellcheck source=/dev/null
    source "${K3SLAB_CLUSTER_PROFILE}"
  fi
}

k3slab_true() {
  case "${1,,}" in
    true|1|yes)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

k3slab_k3s_server_args() {
  k3slab_load_cluster_profile
  local args=(
    --write-kubeconfig-mode 644 \
    --bind-address 0.0.0.0 \
    --https-listen-port 6443 \
    --disable-network-policy \
    --disable=metrics-server \
    --snapshotter="${K3SLAB_K3S_SNAPSHOTTER}"
  )
  if k3slab_true "${K3SLAB_DISABLE_TRAEFIK}"; then
    args+=(--disable=traefik)
  fi
  printf '%s ' "${args[@]}"
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

# Kill workshop/lab processes that can outlive a K3s stop (background setup, helm, ingress).
kill_lab_orphans() {
  local pattern
  local -a patterns=(
    '[n]ginx-ingress-controller'
    '[t]raefik traefik'
    '[/]usr/local/bin/helm'
    '[s]cripts/setup.sh'
    'lb-tcp-'
  )
  for pattern in "${patterns[@]}"; do
    pkill -TERM -f "${pattern}" 2>/dev/null || true
  done
  sleep 1
  for pattern in "${patterns[@]}"; do
    pkill -KILL -f "${pattern}" 2>/dev/null || true
  done
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

# Drop stale containerd sandbox/rootfs mounts under /run/k3s so runtime dirs can be cleared.
unmount_k3s_runtime() {
  unmount_k3s_tmpmounts
  local run_root=/run/k3s/containerd
  [[ -d "${run_root}" ]] || return 0
  local m
  while IFS= read -r -d '' m; do
    umount -l "${m}" 2>/dev/null || true
  done < <(find "${run_root}" \( -name rootfs -o -name shm -o -name merged \) -print0 2>/dev/null)
  while IFS= read -r -d '' m; do
    umount -l "${m}" 2>/dev/null || true
  done < <(find "${run_root}" -type d -name snap -print0 2>/dev/null)
}

_k3slab_iptables_restore_without_k8s() {
  local save_cmd=( "$1" )
  local restore_cmd=( "$2" )
  local table="$3"
  if ! command -v "${save_cmd[0]}" >/dev/null 2>&1; then
    return 0
  fi
  if ! command -v "${restore_cmd[0]}" >/dev/null 2>&1; then
    return 0
  fi
  if ! "${save_cmd[@]}" -t "${table}" &>/dev/null; then
    return 0
  fi
  "${save_cmd[@]}" -t "${table}" 2>/dev/null \
    | grep -vE 'KUBE-|CNI-|FLANNEL-' \
    | "${restore_cmd[@]}" -t "${table}" 2>/dev/null || true
}

# CNI hostport rules can outlive deleted pods and hijack :80/:443 on localhost after lab switches.
flush_stale_cni_hostport() {
  command -v iptables-save >/dev/null 2>&1 || return 0
  command -v iptables >/dev/null 2>&1 || return 0

  local chain
  while IFS= read -r chain; do
    [[ -n "${chain}" ]] || continue
    iptables -t nat -F "${chain}" 2>/dev/null || true
    iptables -t nat -X "${chain}" 2>/dev/null || true
  done < <(iptables-save -t nat 2>/dev/null | sed -n 's/^:\(CNI-DN-[^ ]*\) .*/\1/p')

  for chain in CNI-HOSTPORT-DNAT CNI-HOSTPORT-MASQ CNI-HOSTPORT-SETMARK; do
    iptables -t nat -F "${chain}" 2>/dev/null || true
  done
}

# Remove K3s/CNI/Flannel host networking state so the next cluster boot matches a fresh container.
reset_k3s_host_networking() {
  echo "[k3slab] Resetting host networking..."
  local table iface
  for table in filter nat mangle raw; do
    _k3slab_iptables_restore_without_k8s iptables-save iptables-restore "${table}"
    _k3slab_iptables_restore_without_k8s ip6tables-save ip6tables-restore "${table}"
  done
  flush_stale_cni_hostport
  if command -v conntrack >/dev/null 2>&1; then
    conntrack -F 2>/dev/null || true
  fi
  for iface in cni0 flannel.1; do
    ip link delete "${iface}" 2>/dev/null || true
  done
}

k3slab_dir_is_empty() {
  local path="$1"
  [[ ! -e "${path}" ]] && return 0
  [[ -z "$(ls -A "${path}" 2>/dev/null)" ]]
}

# Remove a path, or clear its contents when it is a mount point (e.g. docker compose volumes).
wipe_k3s_path() {
  local path="$1"
  [[ -e "${path}" ]] || return 0
  if mountpoint -q "${path}" 2>/dev/null; then
    find "${path}" -mindepth 1 -delete 2>/dev/null \
      || rm -rf "${path:?}"/* "${path:?}"/.[!.]* "${path:?}"/..?* 2>/dev/null \
      || return 1
  else
    rm -rf "${path}" 2>/dev/null || return 1
  fi
  k3slab_dir_is_empty "${path}"
}

k3slab_paths_wiped() {
  local path
  for path in "$@"; do
    if ! k3slab_dir_is_empty "${path}"; then
      return 1
    fi
  done
  return 0
}

wipe_k3s_filesystem_state() {
  local -a required=(
    "${K3S_DATA_DIR}"
    /etc/rancher/k3s
  )
  local -a optional=(
    /run/k3s
    /var/lib/cni
    /etc/cni/net.d
  )
  local attempt path
  for attempt in $(seq 1 10); do
    unmount_k3s_runtime
    local ok=1
    for path in "${required[@]}"; do
      if ! wipe_k3s_path "${path}"; then
        ok=0
      fi
    done
    for path in "${optional[@]}"; do
      wipe_k3s_path "${path}" || true
    done
    if [[ "${ok}" -eq 1 ]] && k3slab_paths_wiped "${required[@]}"; then
      return 0
    fi
    echo "[k3slab] wipe attempt ${attempt}/10 failed, retrying..."
    killall_k3s
    kill_lab_orphans
    sleep 2
  done
  echo "[k3slab] ERROR: could not wipe K3s filesystem state"
  return 1
}

# Backwards-compatible alias used by older call sites.
wipe_k3s_data() {
  wipe_k3s_filesystem_state
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
  kill_lab_orphans
  if ! wipe_k3s_filesystem_state; then
    restore_k3s_after_reset_failure || true
    return 1
  fi
  reset_k3s_host_networking
  start_k3s >/dev/null
  wait_ready
}
