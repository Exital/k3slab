#!/usr/bin/env bash
# Print a host-reachable Gitea HTTP remote for git clone/push.
# Usage: remote=$(bash scripts/gitea-http-remote.sh [username] [password])
# Prefer pod/endpoint IP (kube-proxy ClusterIP is flaky after multi-lab resets).
set -euo pipefail

user="${1:-gitops}"
pass="${2:-gitops123}"
repo="${3:-gitops/demo-app.git}"

ip="$(kubectl -n gitops-lab get endpoints gitea -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
if [[ -z "${ip}" ]]; then
  echo "gitea endpoints not ready" >&2
  exit 1
fi

# Probe pod IP from the lab host; fall back to port-forward if needed.
if curl -sf --connect-timeout 2 --max-time 3 "http://${ip}:3000/api/v1/version" >/dev/null 2>&1; then
  printf 'http://%s:%s@%s:3000/%s\n' "${user}" "${pass}" "${ip}" "${repo}"
  exit 0
fi

# Reuse a stable local forward if already running; otherwise start one.
pf_port=31302
if ! curl -sf --connect-timeout 1 --max-time 2 "http://127.0.0.1:${pf_port}/api/v1/version" >/dev/null 2>&1; then
  # shellcheck disable=SC2009
  if ! ps -ef 2>/dev/null | grep -q '[k]ubectl.*port-forward.*svc/gitea.*31302'; then
    kubectl -n gitops-lab port-forward svc/gitea "${pf_port}:3000" >/tmp/gitea-pf-git.log 2>&1 &
    sleep 2
  fi
fi
if ! curl -sf --connect-timeout 2 --max-time 3 "http://127.0.0.1:${pf_port}/api/v1/version" >/dev/null 2>&1; then
  echo "gitea not reachable via pod IP or port-forward" >&2
  exit 1
fi
printf 'http://%s:%s@127.0.0.1:%s/%s\n' "${user}" "${pass}" "${pf_port}" "${repo}"
