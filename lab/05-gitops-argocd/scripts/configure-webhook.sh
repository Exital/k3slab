#!/usr/bin/env bash
# Register a Gitea push webhook that notifies Argo CD immediately (not poll-only).
# Use Gogs-compatible hook type: Argo CD v2.14 accepts X-Gogs-Event (X-Gitea-Event returns 400).
set -euo pipefail

cd "$(dirname "$0")/.."

GITEA_URL="${GITEA_URL:-http://gitea.gitops-lab.svc.cluster.local:3000}"
GITEA_USER="${GITEA_USER:-gitops}"
GITEA_PASS="${GITEA_PASS:-gitops123}"
REPO_NAME="${REPO_NAME:-demo-app}"
WEBHOOK_SECRET="${WEBHOOK_SECRET:-k3slab-gitops-webhook}"
# Proxy rewrites ROOT_URL (localhost/gitea) → ClusterIP URL Argo watches, then forwards.
ARGOCD_WEBHOOK_URL="${ARGOCD_WEBHOOK_URL:-http://gitops-webhook-proxy.gitops-lab.svc.cluster.local:8080/}"

bash scripts/map-cluster-dns.sh

echo "[gitops-lab] Ensuring Argo CD webhook.gogs.secret..."
secret_json="$(jq -n --arg s "${WEBHOOK_SECRET}" '{stringData: {"webhook.gogs.secret": $s, "webhook.gitea.secret": $s}}')"
kubectl -n argocd patch secret argocd-secret --type merge -p "${secret_json}" >/dev/null

hooks_json="$(curl -sf --connect-timeout 3 --max-time 15 \
  -u "${GITEA_USER}:${GITEA_PASS}" \
  "${GITEA_URL}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/hooks")"

# Replace any prior lab hooks aimed at Argo CD.
printf '%s' "${hooks_json}" | jq -r --arg url "${ARGOCD_WEBHOOK_URL}" \
  '.[] | select(.config.url == $url) | .id' | while read -r hid; do
  [[ -z "${hid}" || "${hid}" == "null" ]] && continue
  echo "[gitops-lab] Deleting old webhook id=${hid}..."
  curl -sf --connect-timeout 3 --max-time 15 \
    -u "${GITEA_USER}:${GITEA_PASS}" \
    -X DELETE "${GITEA_URL}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/hooks/${hid}" >/dev/null || true
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
  -X POST "${GITEA_URL}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/hooks" \
  -d "${payload}" >/dev/null

echo "[gitops-lab] Webhook ready: push → ${ARGOCD_WEBHOOK_URL}"
