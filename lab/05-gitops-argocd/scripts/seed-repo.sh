#!/usr/bin/env bash
# Seed Gitea with the half-ready demo-app repo (broken Service selector).
# Platform user `gitops` owns setup; student gets the same login as Argo CD + admin on the repo.
set -euo pipefail

cd "$(dirname "$0")/.."
LAB_ROOT="$(pwd)"

GITEA_USER="${GITEA_USER:-gitops}"
GITEA_PASS="${GITEA_PASS:-gitops123}"
GITEA_EMAIL="${GITEA_EMAIL:-gitops@k3slab.local}"
REPO_NAME="${REPO_NAME:-demo-app}"

raw="${student_username:-${STUDENT_ID:-student}}"
# shellcheck disable=SC2001
STUDENT_ID="$(printf '%s' "${raw}" | sed -E 's/[[:space:]]+/_/g')"
STUDENT_PASS="${STUDENT_ID}"
STUDENT_EMAIL="${STUDENT_ID}@k3slab.local"

bash scripts/map-cluster-dns.sh

# Prefer pod/endpoint IP (headless Service + works when kube-proxy ClusterIP is broken).
gitea_ip="$(kubectl -n gitops-lab get endpoints gitea -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
if [[ -z "${gitea_ip}" ]]; then
  gitea_ip="$(kubectl -n gitops-lab get svc gitea -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
fi
GITEA_URL="${GITEA_URL:-http://${gitea_ip}:3000}"
PF_PID=""
SEED_DIR=""

cleanup() {
  if [[ -n "${PF_PID}" ]]; then
    kill "${PF_PID}" 2>/dev/null || true
  fi
  if [[ -n "${SEED_DIR}" ]]; then
    rm -rf "${SEED_DIR}"
  fi
}
trap cleanup EXIT

# Ingress uses stripPrefix (/gitea → /); container API is at the root.
# Try /api first, then /gitea/api for safety.
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

echo "[gitops-lab] Waiting for Gitea HTTP at ${GITEA_URL}..."
API_BASE=""
for _ in $(seq 1 60); do
  if API_BASE="$(api_version_url "${GITEA_URL}")"; then
    break
  fi
  if kubectl -n gitops-lab exec deploy/gitea -- \
    wget -q -O /dev/null http://127.0.0.1:3000/api/v1/version 2>/dev/null \
    || kubectl -n gitops-lab exec deploy/gitea -- \
      wget -q -O /dev/null http://127.0.0.1:3000/gitea/api/v1/version 2>/dev/null; then
    if [[ -z "${PF_PID}" ]]; then
      echo "[gitops-lab] ClusterIP not reachable from host; using port-forward..."
      local_port=31300
      kubectl -n gitops-lab port-forward svc/gitea "${local_port}:3000" >/tmp/gitea-pf.log 2>&1 &
      PF_PID=$!
      GITEA_URL="http://127.0.0.1:${local_port}"
      sleep 2
    fi
    if API_BASE="$(api_version_url "${GITEA_URL}")"; then
      break
    fi
  fi
  API_BASE=""
  sleep 2
done
if [[ -z "${API_BASE}" ]]; then
  echo "[gitops-lab] Gitea API not reachable at ${GITEA_URL}/api/v1/version" >&2
  kubectl -n gitops-lab get endpoints gitea -o wide >&2 || true
  kubectl -n gitops-lab logs deploy/gitea --tail=40 >&2 || true
  exit 1
fi
echo "[gitops-lab] Gitea API base: ${API_BASE}"

if ! kubectl -n gitops-lab exec deploy/gitea -- \
  su-exec git gitea admin user list 2>/dev/null | awk '{print $2}' | grep -qx "${GITEA_USER}"; then
  echo "[gitops-lab] Creating Gitea platform user ${GITEA_USER}..."
  kubectl -n gitops-lab exec deploy/gitea -- \
    su-exec git gitea admin user create \
      --username "${GITEA_USER}" \
      --password "${GITEA_PASS}" \
      --email "${GITEA_EMAIL}" \
      --admin \
      --must-change-password=false >/dev/null
fi

repo_code=$(curl -s -o /dev/null -w '%{http_code}' --connect-timeout 3 --max-time 10 \
  -u "${GITEA_USER}:${GITEA_PASS}" \
  "${API_BASE}/api/v1/repos/${GITEA_USER}/${REPO_NAME}" || true)
if [[ "${repo_code}" != "200" ]]; then
  echo "[gitops-lab] Creating repo ${GITEA_USER}/${REPO_NAME}..."
  curl -sf --connect-timeout 3 --max-time 15 \
    -u "${GITEA_USER}:${GITEA_PASS}" \
    -H 'Content-Type: application/json' \
    -X POST "${API_BASE}/api/v1/user/repos" \
    -d "{\"name\":\"${REPO_NAME}\",\"private\":false,\"auto_init\":false}" >/dev/null
fi

SEED_DIR="$(mktemp -d /tmp/gitops-seed.XXXXXX)"

if [[ ! -f "${LAB_ROOT}/repo-seed/demo-app/deployment.yml" ]]; then
  echo "[gitops-lab] missing ${LAB_ROOT}/repo-seed/demo-app/deployment.yml" >&2
  exit 1
fi

# Don't use cp -a: preserved host UIDs trigger "dubious ownership" inside the container.
cp -R "${LAB_ROOT}/repo-seed/demo-app/." "${SEED_DIR}/"
chown -R "$(id -u):$(id -g)" "${SEED_DIR}" 2>/dev/null || true
cd "${SEED_DIR}"

git config --global --add safe.directory '*'

git init -q
git checkout -q -b main 2>/dev/null || git symbolic-ref HEAD refs/heads/main
git config user.email "${GITEA_EMAIL}"
git config user.name "${GITEA_USER}"
git add -A
git status --short
git commit -q -m "Initial half-ready demo-app manifests"

# Git HTTP path matches API base (root when stripPrefix is used).
git_base="${API_BASE#http://}"
REMOTE="http://${GITEA_USER}:${GITEA_PASS}@${git_base}/${GITEA_USER}/${REPO_NAME}.git"
echo "[gitops-lab] Pushing seed to ${GITEA_USER}/${REPO_NAME}..."
git push -q --force "${REMOTE}" HEAD:main

if ! git ls-remote "${REMOTE}" | grep -q 'refs/heads/main'; then
  echo "[gitops-lab] seed push failed — main branch missing on remote" >&2
  git ls-remote "${REMOTE}" >&2 || true
  exit 1
fi
echo "[gitops-lab] Remote main is present."

if ! kubectl -n gitops-lab exec deploy/gitea -- \
  su-exec git gitea admin user list 2>/dev/null | awk '{print $2}' | grep -qx "${STUDENT_ID}"; then
  echo "[gitops-lab] Creating Gitea student user ${STUDENT_ID} (same credentials as Argo CD)..."
  kubectl -n gitops-lab exec deploy/gitea -- \
    su-exec git gitea admin user create \
      --username "${STUDENT_ID}" \
      --password "${STUDENT_PASS}" \
      --email "${STUDENT_EMAIL}" \
      --must-change-password=false >/dev/null
fi

echo "[gitops-lab] Granting ${STUDENT_ID} admin on ${GITEA_USER}/${REPO_NAME}..."
curl -sf --connect-timeout 3 --max-time 15 \
  -u "${GITEA_USER}:${GITEA_PASS}" \
  -H 'Content-Type: application/json' \
  -X PUT "${API_BASE}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/collaborators/${STUDENT_ID}" \
  -d '{"permission":"admin"}' >/dev/null

echo "[gitops-lab] Seeded ${GITEA_USER}/${REPO_NAME}; student login ${STUDENT_ID}/${STUDENT_ID} has admin on the repo."
