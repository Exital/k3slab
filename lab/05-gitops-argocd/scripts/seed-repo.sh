#!/usr/bin/env bash
# Seed Gitea with the half-ready demo-app repo (broken Service selector).
# Platform user `gitops` owns setup; student gets the same login as Argo CD + admin on the repo.
set -euo pipefail

cd "$(dirname "$0")/.."
LAB_ROOT="$(pwd)"

GITEA_URL="${GITEA_URL:-http://gitea.gitops-lab.svc.cluster.local:3000}"
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

echo "[gitops-lab] Waiting for Gitea HTTP at ${GITEA_URL}..."
ok=0
for _ in $(seq 1 90); do
  if curl -sf --connect-timeout 3 --max-time 5 "${GITEA_URL}/api/v1/version" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 2
done
if [[ "${ok}" != "1" ]]; then
  echo "[gitops-lab] Gitea API not reachable at ${GITEA_URL}/api/v1/version" >&2
  exit 1
fi

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
  "${GITEA_URL}/api/v1/repos/${GITEA_USER}/${REPO_NAME}" || true)
if [[ "${repo_code}" != "200" ]]; then
  echo "[gitops-lab] Creating repo ${GITEA_USER}/${REPO_NAME}..."
  curl -sf --connect-timeout 3 --max-time 15 \
    -u "${GITEA_USER}:${GITEA_PASS}" \
    -H 'Content-Type: application/json' \
    -X POST "${GITEA_URL}/api/v1/user/repos" \
    -d "{\"name\":\"${REPO_NAME}\",\"private\":false,\"auto_init\":false}" >/dev/null
fi

SEED_DIR="$(mktemp -d /tmp/gitops-seed.XXXXXX)"
cleanup() { rm -rf "${SEED_DIR}"; }
trap cleanup EXIT

if [[ ! -f "${LAB_ROOT}/repo-seed/demo-app/deployment.yml" ]]; then
  echo "[gitops-lab] missing ${LAB_ROOT}/repo-seed/demo-app/deployment.yml" >&2
  exit 1
fi

# Don't use cp -a: preserved host UIDs trigger "dubious ownership" inside the container.
cp -R "${LAB_ROOT}/repo-seed/demo-app/." "${SEED_DIR}/"
chown -R "$(id -u):$(id -g)" "${SEED_DIR}" 2>/dev/null || true
cd "${SEED_DIR}"

# Extra safety for Git ≥2.35 ownership checks in container/lab shells.
git config --global --add safe.directory '*'

git init -q
# Compatible with older git (no `git init -b`).
git checkout -q -b main 2>/dev/null || git symbolic-ref HEAD refs/heads/main
git config user.email "${GITEA_EMAIL}"
git config user.name "${GITEA_USER}"
git add -A
git status --short
git commit -q -m "Initial half-ready demo-app manifests"

REMOTE="http://${GITEA_USER}:${GITEA_PASS}@gitea.gitops-lab.svc.cluster.local:3000/${GITEA_USER}/${REPO_NAME}.git"
echo "[gitops-lab] Pushing seed to ${GITEA_USER}/${REPO_NAME}..."
git push -q --force "${REMOTE}" HEAD:main

# Verify the push actually landed (ClusterIP contents API can lie without Host; use ls-remote).
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
  -X PUT "${GITEA_URL}/api/v1/repos/${GITEA_USER}/${REPO_NAME}/collaborators/${STUDENT_ID}" \
  -d '{"permission":"admin"}' >/dev/null

echo "[gitops-lab] Seeded ${GITEA_USER}/${REPO_NAME}; student login ${STUDENT_ID}/${STUDENT_ID} has admin on the repo."
