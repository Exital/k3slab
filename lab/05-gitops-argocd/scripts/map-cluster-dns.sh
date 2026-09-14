#!/usr/bin/env bash
# Map in-cluster Service DNS names into /etc/hosts for the lab host shell
# (K3sLab runs kubectl/git/curl in the container netns, which does not use CoreDNS).
# Note: Docker often makes `sed -i /etc/hosts` fail ("Device or resource busy"); append only.
set -euo pipefail

upsert_host() {
  local ip="$1"
  local name="$2"
  if grep -qE "[[:space:]]${name}([[:space:]]|$)" /etc/hosts 2>/dev/null; then
    if grep -qE "^${ip}[[:space:]].*[[:space:]]${name}([[:space:]]|$)|^${ip}[[:space:]]+${name}$" /etc/hosts 2>/dev/null; then
      return 0
    fi
    printf '%s %s\n' "${ip}" "${name}" >>/etc/hosts
    echo "[gitops-lab] /etc/hosts: appended ${ip} ${name} (prior mapping may be stale)"
    return 0
  fi
  printf '%s %s\n' "${ip}" "${name}" >>/etc/hosts
}

# Prefer Endpoints/pod IP (works for headless Services and when kube-proxy ClusterIP is broken).
svc_reach_ip() {
  local ns="$1"
  local svc="$2"
  local ip=""
  ip="$(kubectl -n "${ns}" get endpoints "${svc}" -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
  if [[ -n "${ip}" ]]; then
    printf '%s\n' "${ip}"
    return 0
  fi
  ip="$(kubectl -n "${ns}" get svc "${svc}" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
  if [[ -n "${ip}" && "${ip}" != "None" ]]; then
    printf '%s\n' "${ip}"
    return 0
  fi
  return 1
}

map_svc() {
  local ns="$1"
  local svc="$2"
  local name="${3-}"
  if [[ -z "${name}" ]]; then
    name="${svc}.${ns}.svc.cluster.local"
  fi
  local ip
  if ! ip="$(svc_reach_ip "${ns}" "${svc}")"; then
    echo "[gitops-lab] warn: no reachable IP for ${ns}/${svc}" >&2
    return 0
  fi
  upsert_host "${ip}" "${name}"
  echo "[gitops-lab] /etc/hosts: ${ip} ${name}"
}

map_svc gitops-lab gitea
map_svc argocd argocd-server || true
