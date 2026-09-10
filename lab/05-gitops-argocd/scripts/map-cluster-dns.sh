#!/usr/bin/env bash
# Map in-cluster Service DNS names into /etc/hosts for the lab host shell
# (K3sLab runs kubectl/git/curl in the container netns, which does not use CoreDNS).
# Note: Docker often makes `sed -i /etc/hosts` fail ("Device or resource busy"); append only.
set -euo pipefail

upsert_host() {
  local ip="$1"
  local name="$2"
  if grep -qE "[[:space:]]${name}([[:space:]]|$)" /etc/hosts 2>/dev/null; then
    # Already present (possibly stale IP). Leave it — ClusterIPs are stable for the lab lifetime.
    return 0
  fi
  printf '%s %s\n' "${ip}" "${name}" >>/etc/hosts
}

map_svc() {
  local ns="$1"
  local svc="$2"
  local name="${3-}"
  if [[ -z "${name}" ]]; then
    name="${svc}.${ns}.svc.cluster.local"
  fi
  local ip
  ip="$(kubectl -n "${ns}" get svc "${svc}" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
  if [[ -z "${ip}" || "${ip}" == "None" ]]; then
    echo "[gitops-lab] warn: no ClusterIP for ${ns}/${svc}" >&2
    return 0
  fi
  upsert_host "${ip}" "${name}"
  echo "[gitops-lab] /etc/hosts: ${ip} ${name}"
}

map_svc gitops-lab gitea
map_svc argocd argocd-server || true
