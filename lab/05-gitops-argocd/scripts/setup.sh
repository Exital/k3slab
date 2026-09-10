#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export K3SLAB_INGRESS_HOST="${K3SLAB_INGRESS_HOST:-localhost}"

# Personalized Argo CD login: username == password == normalized student_username.
raw="${student_username:-student}"
# shellcheck disable=SC2001
STUDENT_ID="$(printf '%s' "${raw}" | sed -E 's/[[:space:]]+/_/g')"
export STUDENT_ID
echo "[gitops-lab] Argo CD student identity: ${STUDENT_ID} (user=password)"

ARGOCD_CHART_VERSION="${ARGOCD_CHART_VERSION:-7.9.1}"
ARGOCD_APP_VERSION="${ARGOCD_APP_VERSION:-v2.14.11}"
GITEA_IMAGE="${GITEA_IMAGE:-docker.io/gitea/gitea:1.22.6}"
NGINX_IMAGE="${NGINX_IMAGE:-docker.io/library/nginx:1.27-alpine}"
REDIS_IMAGE="${REDIS_IMAGE:-public.ecr.aws/docker/library/redis:7.2.8-alpine}"

echo "[gitops-lab] Applying namespaces..."
kubectl apply -f manifests/00-namespace.yml

# Namespace controller creates the default SA asynchronously.
for _ in $(seq 1 60); do
  if kubectl -n gitops-lab get sa default >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
kubectl -n gitops-lab get sa default >/dev/null

echo "[gitops-lab] Pre-pulling images (Gitea, nginx, redis, Argo CD)..."
k3s ctr images pull "${GITEA_IMAGE}"
k3s ctr images pull "${NGINX_IMAGE}"
k3s ctr images pull "${REDIS_IMAGE}"
k3s ctr images pull "quay.io/argoproj/argocd:${ARGOCD_APP_VERSION}"
k3s ctr images pull "docker.io/library/python:3.12-alpine"

echo "[gitops-lab] Installing Gitea..."
# Patch ROOT_URL / DOMAIN for the active ingress host before apply.
host_esc="${K3SLAB_INGRESS_HOST//\//\\/}"
tmp_gitea="$(mktemp)"
sed \
  -e "s|value: \"localhost\"|value: \"${K3SLAB_INGRESS_HOST}\"|" \
  -e "s|value: \"http://localhost/gitea/\"|value: \"http://${host_esc}/gitea/\"|" \
  manifests/gitea.yml >"${tmp_gitea}"
kubectl apply -f "${tmp_gitea}"
rm -f "${tmp_gitea}"

# Prefer rendered ingress (platform templates); always inject active ingress host.
kubectl apply -f manifests/gitea-stripprefix.yml
tmp_gitea_ing="$(mktemp)"
if [[ -f manifests/gitea-ingress.yml.template ]]; then
  export K3SLAB_INGRESS_HOST
  envsubst '${K3SLAB_INGRESS_HOST}' <manifests/gitea-ingress.yml.template >"${tmp_gitea_ing}"
elif [[ -f manifests/gitea-ingress.yml ]]; then
  sed "s/host: localhost/host: ${K3SLAB_INGRESS_HOST}/" manifests/gitea-ingress.yml >"${tmp_gitea_ing}"
else
  echo "[gitops-lab] missing gitea ingress manifest" >&2
  exit 1
fi
kubectl apply -f "${tmp_gitea_ing}"
rm -f "${tmp_gitea_ing}"

kubectl -n gitops-lab rollout status deploy/gitea --timeout=300s
kubectl -n gitops-lab wait --for=condition=Ready pod -l app=gitea --timeout=300s

# Host shell cannot resolve *.svc.cluster.local via CoreDNS — map ClusterIPs.
bash scripts/map-cluster-dns.sh

echo "[gitops-lab] Seeding Gitea repo..."
bash scripts/seed-repo.sh

echo "[gitops-lab] Installing Argo CD (Helm ${ARGOCD_CHART_VERSION})..."

# bcrypt hash for student password (same as username).
kubectl -n argocd delete pod bcrypt-gen --ignore-not-found >/dev/null 2>&1 || true
kubectl -n argocd run bcrypt-gen --restart=Never \
  --image="quay.io/argoproj/argocd:${ARGOCD_APP_VERSION}" \
  --overrides='{"spec":{"securityContext":{"runAsUser":999}}}' \
  --command -- argocd account bcrypt --password "${STUDENT_ID}" >/dev/null
for _ in $(seq 1 90); do
  phase=$(kubectl -n argocd get pod bcrypt-gen -o jsonpath='{.status.phase}' 2>/dev/null || true)
  if [[ "${phase}" == "Succeeded" || "${phase}" == "Failed" ]]; then
    break
  fi
  sleep 1
done
BCRYPT_HASH="$(kubectl -n argocd logs bcrypt-gen 2>/dev/null | tr -d '\r' | grep -E '^\$2[aby]\$' | tail -n 1 || true)"
kubectl -n argocd delete pod bcrypt-gen --ignore-not-found >/dev/null 2>&1 || true
if [[ -z "${BCRYPT_HASH}" || "${BCRYPT_HASH}" != \$2* ]]; then
  echo "[gitops-lab] Failed to generate bcrypt hash for student password" >&2
  exit 1
fi

# Argo watches the in-cluster Gitea URL; Gitea webhooks advertise ROOT_URL (ingress host).
# webhook-proxy rewrites the payload host so Argo matches the Application.
echo "[gitops-lab] Installing Gitea → Argo webhook URL rewrite proxy..."
tmp_proxy="$(mktemp)"
# Escape sed replacement specials in ingress host.
proxy_from="http://${K3SLAB_INGRESS_HOST}/gitea"
proxy_from_esc="$(printf '%s' "${proxy_from}" | sed -e 's/[&|\\]/\\&/g')"
sed -e "s|value: http://localhost/gitea|value: ${proxy_from_esc}|" \
  manifests/webhook-proxy.yml >"${tmp_proxy}"
kubectl apply -f "${tmp_proxy}"
rm -f "${tmp_proxy}"
kubectl -n gitops-lab rollout status deploy/gitops-webhook-proxy --timeout=180s

VALUES_FILE="$(mktemp)"
cat >"${VALUES_FILE}" <<EOF
dex:
  enabled: false
notifications:
  enabled: false
applicationSet:
  enabled: false
controller:
  replicas: 1
repoServer:
  replicas: 1
configs:
  params:
    server.insecure: "true"
    server.rootpath: "/argocd"
    server.basehref: "/argocd"
  cm:
    accounts.${STUDENT_ID}: login
    # Backup only — Gitea push webhook triggers immediate refresh (see configure-webhook.sh).
    timeout.reconciliation: 180s
  rbac:
    policy.csv: |
      g, ${STUDENT_ID}, role:readonly
    policy.default: role:readonly
  secret:
    extra:
      accounts.${STUDENT_ID}.password: '${BCRYPT_HASH}'
      webhook.gogs.secret: k3slab-gitops-webhook
      webhook.gitea.secret: k3slab-gitops-webhook
redis:
  image:
    repository: public.ecr.aws/docker/library/redis
    tag: 7.2.8-alpine
server:
  replicas: 1
  service:
    type: ClusterIP
EOF

helm upgrade --install argocd argo-cd \
  --repo https://argoproj.github.io/argo-helm \
  --namespace argocd \
  --version "${ARGOCD_CHART_VERSION}" \
  --values "${VALUES_FILE}" \
  --wait --timeout 8m
rm -f "${VALUES_FILE}"

tmp_argocd_ing="$(mktemp)"
if [[ -f manifests/argocd-ingress.yml.template ]]; then
  export K3SLAB_INGRESS_HOST
  envsubst '${K3SLAB_INGRESS_HOST}' <manifests/argocd-ingress.yml.template >"${tmp_argocd_ing}"
elif [[ -f manifests/argocd-ingress.yml ]]; then
  sed "s/host: localhost/host: ${K3SLAB_INGRESS_HOST}/" manifests/argocd-ingress.yml >"${tmp_argocd_ing}"
else
  echo "[gitops-lab] missing argocd ingress manifest" >&2
  exit 1
fi
kubectl apply -f "${tmp_argocd_ing}"
rm -f "${tmp_argocd_ing}"

kubectl -n argocd rollout status deploy/argocd-server --timeout=300s
kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=300s
kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=300s 2>/dev/null \
  || kubectl -n argocd rollout status deploy/argocd-application-controller --timeout=300s

# Ensure student account exists even if Helm secret merge skipped a field.
kubectl -n argocd patch configmap argocd-cm --type merge \
  -p "{\"data\":{\"accounts.${STUDENT_ID}\":\"login\"}}" >/dev/null
patch_json="$(jq -n --arg u "${STUDENT_ID}" --arg p "${BCRYPT_HASH}" \
  '{stringData: {("accounts." + $u + ".password"): $p}}')"
kubectl -n argocd patch secret argocd-secret --type merge -p "${patch_json}" >/dev/null
rbac_json="$(jq -n --arg u "${STUDENT_ID}" \
  '{data: {"policy.csv": ("g, " + $u + ", role:readonly\n"), "policy.default": "role:readonly"}}')"
kubectl -n argocd patch configmap argocd-rbac-cm --type merge -p "${rbac_json}" >/dev/null
kubectl -n argocd rollout restart deploy/argocd-server >/dev/null
kubectl -n argocd rollout status deploy/argocd-server --timeout=180s

bash scripts/map-cluster-dns.sh

echo "[gitops-lab] Registering repo + Application..."
kubectl apply -f manifests/application.yml

echo "[gitops-lab] Configuring Gitea → Argo CD webhook..."
bash scripts/configure-webhook.sh

# Wait until Argo has synced once (Service selector bug => no endpoints is expected).
echo "[gitops-lab] Waiting for Application demo-app to appear and sync..."
for _ in $(seq 1 36); do
  sync=$(kubectl -n argocd get application demo-app -o jsonpath='{.status.sync.status}' 2>/dev/null || true)
  if [[ "${sync}" == "Synced" || "${sync}" == "OutOfSync" ]]; then
    break
  fi
  sleep 5
done
kubectl -n argocd get application demo-app >/dev/null

# Wait for Deployment to exist from the first sync.
for _ in $(seq 1 36); do
  if kubectl -n gitops-lab get deploy demo-app >/dev/null 2>&1; then
    break
  fi
  sleep 5
done
kubectl -n gitops-lab rollout status deploy/demo-app --timeout=180s || true

echo "[gitops-lab] Waiting for Argo CD UI at /argocd..."
host="${K3SLAB_INGRESS_HOST}"
for _ in $(seq 1 40); do
  if curl -sf -H "Host: ${host}" "http://127.0.0.1/argocd/" >/dev/null 2>&1 \
    || curl -sf -H "Host: ${host}" "http://127.0.0.1/argocd" >/dev/null 2>&1; then
    break
  fi
  sleep 3
done

kubectl config set-context --current --namespace=gitops-lab >/dev/null

echo "[gitops-lab] Ready."
echo "[gitops-lab] Argo CD UI: http://${K3SLAB_INGRESS_HOST}/argocd/  (user=${STUDENT_ID} pass=${STUDENT_ID})"
echo "[gitops-lab] Gitea:      http://${K3SLAB_INGRESS_HOST}/gitea/"
echo "[gitops-lab] Git remote (in-cluster): http://<student>:<student>@gitea.gitops-lab.svc.cluster.local:3000/gitops/demo-app.git"
echo "[gitops-lab] Sync: Gitea push webhook → Argo CD (poll backup every 180s)"
