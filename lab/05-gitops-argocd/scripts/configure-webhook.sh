#!/usr/bin/env bash
# Register a Gitea push webhook that notifies Argo CD immediately (not poll-only).
# Use Gogs-compatible hook type: Argo CD v2.14 accepts X-Gogs-Event (X-Gitea-Event returns 400).
set -euo pipefail

cd "$(dirname "$0")/.."

GITEA_USER="${GITEA_USER:-gitops}"
GITEA_PASS="${GITEA_PASS:-gitops123}"
REPO_NAME="${REPO_NAME:-demo-app}"
WEBHOOK_SECRET="${WEBHOOK_SECRET:-k3slab-gitops-webhook}"
ARGOCD_WEBHOOK_URL="${ARGOCD_WEBHOOK_URL:-http://gitops-webhook-proxy.gitops-lab.svc.cluster.local:8080/}"

bash scripts/map-cluster-dns.sh

gitea_ip="$(kubectl -n gitops-lab get endpoints gitea -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
if [[ -z "${gitea_ip}" ]]; then
  gitea_ip="$(kubectl -n gitops-lab get svc gitea -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
fi
GITEA_URL="${GITEA_URL:-http://${gitea_ip}:3000}"
PF_PID=""

cleanup() {
  if [[ -n "${PF_PID}" ]]; then
    kill "${PF_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

api_version_url() {
  local base="$1"
  if curl -sf --connect-timeout 3 --max-time 5 "${base}/api/v1/version" >/dev/null 2>&1; then
    printf '%s\n' "${base}"
    return 0
  fi
  if curl -sf --connect-timeout 3 --max-time 5 "${base}/gitea/api/v1/version" >/dev/null 2>&1; then
    printf '%s\n' "${base}/gitea"
    return 0
  fi
  return 1
}

API_BASE=""
if ! API_BASE="$(api_version_url "${GITEA_URL}")"; then
  echo "[gitops-lab] ClusterIP not reachable from host for webhook setup; using port-forward..."
  local_port=31301
  kubectl -n gitops-lab port-forward svc/gitea "${local_port}:3000" >/tmp/gitea-pf-webhook.log 2>&1 &
  PF_PID=$!
  GITEA_URL="http://127.0.0.1:${local_port}"
  sleep 2
  for _ in $(seq 1 30); do
    if API_BASE="$(api_version_url "${GITEA_URL}")"; then
      break
    fi
    API_BASE=""
    sleep 1
  done
fi
if [[ -z "${API_BASE}" ]]; then
  echo "[gitops-lab] Gitea API not reachable via port-forward for webhook setup" >&2
  exit 1
fi

echo "[gitops-lab] Ensuring Argo CD webhook.gogs.secret..."
secret_json="$(jq -n --arg s "${WEBHOOK_SECRET}" '{stringData: {"webhook.gogs.secret": $s, "webhook.gitea.secret": $s}}')"
kubectl -n argocd patch secret argocd-secret --type merge -p "${secret_json}" >/dev/null

hooks_json="$(curl -sf --connect-timeout 3 --max-time 15 \
  -u "${GITEA_USER}:${GITEA_PASS}" \
  "${API_BASE}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/hooks")"

printf '%s' "${hooks_json}" | jq -r --arg url "${ARGOCD_WEBHOOK_URL}" \
  '.[] | select(.config.url == $url) | .id' | while read -r hid; do
  [[ -z "${hid}" || "${hid}" == "null" ]] && continue
  echo "[gitops-lab] Deleting old webhook id=${hid}..."
  curl -sf --connect-timeout 3 --max-time 15 \
    -u "${GITEA_USER}:${GITEA_PASS}" \
    -X DELETE "${API_BASE}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/hooks/${hid}" >/dev/null || true
done

payload="$(jq -n \
  --arg url "${ARGOCD_WEBHOOK_URL}" \
  --arg secret "${WEBHOOK_SECRET}" \
  '{
    type: "gogs",
    active: true,
    events: ["push"],
    config: {
      url: $url,
      content_type: "json",
      secret: $secret
    }
  }')"

echo "[gitops-lab] Creating Gitea → Argo CD push webhook (gogs-compatible)..."
curl -sf --connect-timeout 3 --max-time 15 \
  -u "${GITEA_USER}:${GITEA_PASS}" \
  -H 'Content-Type: application/json' \
  -X POST "${API_BASE}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/hooks" \
  -d "${payload}" >/dev/null

echo "[gitops-lab] Webhook ready: push → ${ARGOCD_WEBHOOK_URL}"
