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
PYTHON_IMAGE="${PYTHON_IMAGE:-docker.io/library/python:3.12-alpine}"
ARGOCD_IMAGE="quay.io/argoproj/argocd:${ARGOCD_APP_VERSION}"

# --- helpers -----------------------------------------------------------------

bcrypt_password() {
  local pass="$1"
  local hash=""

  # Fast path: glibc/libxcrypt via Python (available in the lab Ubuntu image).
  hash="$(
    PASSWORD="${pass}" python3 - <<'PY' 2>/dev/null || true
import crypt
import os
import sys

pw = os.environ["PASSWORD"]
try:
    # rounds is 2**cost; 1024 => $2b$10$ (Argo CD default cost).
    h = crypt.crypt(pw, crypt.mksalt(crypt.METHOD_BLOWFISH, rounds=1024))
except Exception:
    sys.exit(1)
if not h or not h.startswith("$2"):
    sys.exit(1)
print(h)
PY
  )"
  if [[ -n "${hash}" && "${hash}" == \$2* ]]; then
    printf '%s\n' "${hash}"
    return 0
  fi

  echo "[gitops-lab] Local bcrypt unavailable; using Argo CD bcrypt pod..." >&2
  kubectl -n argocd delete pod bcrypt-gen --ignore-not-found >/dev/null 2>&1 || true
  kubectl -n argocd run bcrypt-gen --restart=Never \
    --image="${ARGOCD_IMAGE}" \
    --overrides='{"spec":{"securityContext":{"runAsUser":999}}}' \
    --command -- argocd account bcrypt --password "${pass}" >/dev/null
  for _ in $(seq 1 90); do
    phase=$(kubectl -n argocd get pod bcrypt-gen -o jsonpath='{.status.phase}' 2>/dev/null || true)
    if [[ "${phase}" == "Succeeded" || "${phase}" == "Failed" ]]; then
      break
    fi
    sleep 1
  done
  hash="$(kubectl -n argocd logs bcrypt-gen 2>/dev/null | tr -d '\r' | grep -E '^\$2[aby]\$' | tail -n 1 || true)"
  kubectl -n argocd delete pod bcrypt-gen --ignore-not-found >/dev/null 2>&1 || true
  if [[ -z "${hash}" || "${hash}" != \$2* ]]; then
    echo "[gitops-lab] Failed to generate bcrypt hash for student password" >&2
    return 1
  fi
  printf '%s\n' "${hash}"
}

apply_gitea() {
  echo "[gitops-lab] Installing Gitea..."
  host_esc="${K3SLAB_INGRESS_HOST//\//\\/}"
  tmp_gitea="$(mktemp)"
  sed \
    -e "s|value: \"localhost\"|value: \"${K3SLAB_INGRESS_HOST}\"|" \
    -e "s|value: \"http://localhost/gitea/\"|value: \"http://${host_esc}/gitea/\"|" \
    manifests/gitea.yml >"${tmp_gitea}"
  kubectl apply -f "${tmp_gitea}"
  rm -f "${tmp_gitea}"

  # Traefik CRDs can lag briefly after a fresh cluster / lab reset.
  for _ in $(seq 1 60); do
    if kubectl get crd middlewares.traefik.io >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  kubectl wait --for=condition=Established crd/middlewares.traefik.io --timeout=120s >/dev/null
  kubectl apply -f manifests/gitea-stripprefix.yml
  tmp_gitea_ing="$(mktemp)"
  if [[ -f manifests/gitea-ingress.yml.template ]]; then
    export K3SLAB_INGRESS_HOST
    envsubst '${K3SLAB_INGRESS_HOST}' <manifests/gitea-ingress.yml.template >"${tmp_gitea_ing}"
  elif [[ -f manifests/gitea-ingress.yml ]]; then
    sed "s/host: localhost/host: ${K3SLAB_INGRESS_HOST}/" manifests/gitea-ingress.yml >"${tmp_gitea_ing}"
  else
    echo "[gitops-lab] missing gitea ingress manifest" >&2
    return 1
  fi
  kubectl apply -f "${tmp_gitea_ing}"
  rm -f "${tmp_gitea_ing}"

  # Prefer in-pod readiness (exec nc) — then confirm ClusterIP API for seeding.
  if ! kubectl -n gitops-lab rollout status deploy/gitea --timeout=900s; then
    echo "[gitops-lab] Gitea rollout failed; diagnostics:" >&2
    kubectl -n gitops-lab get pods -l app=gitea -o wide >&2 || true
    kubectl -n gitops-lab describe deploy/gitea >&2 || true
    kubectl -n gitops-lab describe pod -l app=gitea >&2 || true
    kubectl -n gitops-lab logs -l app=gitea --tail=80 >&2 || true
    return 1
  fi
  kubectl -n gitops-lab wait --for=condition=Ready pod -l app=gitea --timeout=120s

  bash scripts/map-cluster-dns.sh
  echo "[gitops-lab] Seeding Gitea repo..."
  bash scripts/seed-repo.sh
}

apply_webhook_proxy() {
  echo "[gitops-lab] Installing Gitea → Argo webhook URL rewrite proxy..."
  tmp_proxy="$(mktemp)"
  proxy_from="http://${K3SLAB_INGRESS_HOST}/gitea"
  proxy_from_esc="$(printf '%s' "${proxy_from}" | sed -e 's/[&|\\]/\\&/g')"
  sed -e "s|value: http://localhost/gitea|value: ${proxy_from_esc}|" \
    manifests/webhook-proxy.yml >"${tmp_proxy}"
  kubectl apply -f "${tmp_proxy}"
  rm -f "${tmp_proxy}"
}

install_argocd() {
  local bcrypt_hash="$1"

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
      accounts.${STUDENT_ID}.password: '${bcrypt_hash}'
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

  echo "[gitops-lab] Installing Argo CD (Helm ${ARGOCD_CHART_VERSION})..."
  # Do not --wait on Ready: after multi-lab resets, kubelet→pod probes are flaky even
  # when containers are healthy (host/ClusterIP path). Wait for Running instead.
  helm upgrade --install argocd argo-cd \
    --repo https://argoproj.github.io/argo-helm \
    --namespace argocd \
    --version "${ARGOCD_CHART_VERSION}" \
    --values "${VALUES_FILE}" \
    --timeout 15m
  rm -f "${VALUES_FILE}"

  # Require the components the Application controller talks to — a generic
  # "3 Running pods" check can pass before repo-server is up (CI flake:
  # ComparisonError lookup argocd-repo-server: i/o timeout).
  echo "[gitops-lab] Waiting for Argo CD core components to be Running..."
  ok=0
  for _ in $(seq 1 180); do
    rs_phase="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-repo-server --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    srv_phase="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-server --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    ctrl_phase="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-application-controller --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    redis_phase="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-redis --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    rs_ep="$(kubectl -n argocd get endpoints argocd-repo-server -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)"
    if [[ "${rs_phase}" == "Running" && "${srv_phase}" == "Running" && "${ctrl_phase}" == "Running" && "${redis_phase}" == "Running" && -n "${rs_ep}" ]]; then
      ok=1
      break
    fi
    sleep 2
  done
  if [[ "${ok}" != "1" ]]; then
    echo "[gitops-lab] Argo CD core components not Running in time (repo=${rs_phase:-?} server=${srv_phase:-?} ctrl=${ctrl_phase:-?} redis=${redis_phase:-?} ep=${rs_ep:-none})" >&2
    kubectl -n argocd get pods -o wide >&2 || true
    kubectl -n argocd get endpoints argocd-repo-server -o wide >&2 || true
    return 1
  fi

  # Give CoreDNS a moment after the Argo pod storm (GHA runners get lookup timeouts otherwise).
  echo "[gitops-lab] Waiting for CoreDNS to be Running..."
  kubectl -n kube-system wait --for=condition=Ready pod -l k8s-app=kube-dns --timeout=120s >/dev/null 2>&1 || true
  sleep 3

  # Bypass CoreDNS for controller → repo-server (CI: lookup argocd-repo-server i/o timeout).
  # Argo reads ARGOCD_APPLICATION_CONTROLLER_REPO_SERVER from argocd-cmd-params-cm key repo.server
  # (not ARGOCD_REPOSERVER_ADDRESS).
  rs_addr="$(kubectl -n argocd get svc argocd-repo-server -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
  if [[ -n "${rs_addr}" && "${rs_addr}" != "None" ]]; then
    echo "[gitops-lab] Pinning repo.server to ${rs_addr}:8081 (avoid CoreDNS)..."
    kubectl -n argocd patch configmap argocd-cmd-params-cm --type merge \
      -p "{\"data\":{\"repo.server\":\"${rs_addr}:8081\"}}" >/dev/null
    if kubectl -n argocd get statefulset argocd-application-controller >/dev/null 2>&1; then
      kubectl -n argocd rollout restart statefulset/argocd-application-controller >/dev/null
      kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=180s >/dev/null || true
    elif kubectl -n argocd get deploy argocd-application-controller >/dev/null 2>&1; then
      kubectl -n argocd rollout restart deploy/argocd-application-controller >/dev/null
      kubectl -n argocd rollout status deploy/argocd-application-controller --timeout=180s >/dev/null || true
    fi
    for _ in $(seq 1 60); do
      ctrl_phase="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-application-controller --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
      if [[ "${ctrl_phase}" == "Running" ]]; then
        break
      fi
      sleep 2
    done
  else
    echo "[gitops-lab] Warning: could not resolve argocd-repo-server ClusterIP" >&2
  fi

  tmp_argocd_ing="$(mktemp)"
  if [[ -f manifests/argocd-ingress.yml.template ]]; then
    export K3SLAB_INGRESS_HOST
    envsubst '${K3SLAB_INGRESS_HOST}' <manifests/argocd-ingress.yml.template >"${tmp_argocd_ing}"
  elif [[ -f manifests/argocd-ingress.yml ]]; then
    sed "s/host: localhost/host: ${K3SLAB_INGRESS_HOST}/" manifests/argocd-ingress.yml >"${tmp_argocd_ing}"
  else
    echo "[gitops-lab] missing argocd ingress manifest" >&2
    return 1
  fi
  kubectl apply -f "${tmp_argocd_ing}"
  rm -f "${tmp_argocd_ing}"

  # Helm values already set the student account; patch only as a merge safety net (no restart).
  kubectl -n argocd patch configmap argocd-cm --type merge \
    -p "{\"data\":{\"accounts.${STUDENT_ID}\":\"login\"}}" >/dev/null
  patch_json="$(jq -n --arg u "${STUDENT_ID}" --arg p "${bcrypt_hash}" \
    '{stringData: {("accounts." + $u + ".password"): $p}}')"
  kubectl -n argocd patch secret argocd-secret --type merge -p "${patch_json}" >/dev/null
  rbac_json="$(jq -n --arg u "${STUDENT_ID}" \
    '{data: {"policy.csv": ("g, " + $u + ", role:readonly\n"), "policy.default": "role:readonly"}}')"
  kubectl -n argocd patch configmap argocd-rbac-cm --type merge -p "${rbac_json}" >/dev/null
}

# --- main --------------------------------------------------------------------

progress() {
  local pct="$1"
  shift
  if command -v k3slab-progress >/dev/null 2>&1; then
    k3slab-progress "${pct}" "$@"
  else
    # Fallback when the helper is not on PATH (local/dev).
    printf '::k3slab-progress::%s::%s\n' "${pct}" "$*"
  fi
}

progress 5 "Applying namespaces"
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

progress 15 "Pre-pulling images"
echo "[gitops-lab] Pre-pulling images in parallel (Gitea, nginx, redis, Argo CD, python)..."
pull_pids=()
for img in "${GITEA_IMAGE}" "${NGINX_IMAGE}" "${REDIS_IMAGE}" "${ARGOCD_IMAGE}" "${PYTHON_IMAGE}"; do
  k3s ctr images pull "${img}" &
  pull_pids+=("$!")
done
pull_ec=0
for pid in "${pull_pids[@]}"; do
  wait "${pid}" || pull_ec=1
done
if [[ "${pull_ec}" -ne 0 ]]; then
  echo "[gitops-lab] One or more image pulls failed" >&2
  exit 1
fi

# Parallel tracks: Gitea (apply + seed) alongside Argo CD install.
progress 35 "Installing Gitea and Argo CD in parallel"
gitea_ec=0
argo_ec=0
(
  apply_gitea
) &
gitea_pid=$!

(
  echo "[gitops-lab] Generating student password hash..."
  BCRYPT_HASH="$(bcrypt_password "${STUDENT_ID}")"
  apply_webhook_proxy
  kubectl -n gitops-lab rollout status deploy/gitops-webhook-proxy --timeout=180s &
  proxy_wait_pid=$!
  install_argocd "${BCRYPT_HASH}"
  wait "${proxy_wait_pid}"
) &
argo_pid=$!

wait "${gitea_pid}" || gitea_ec=$?
wait "${argo_pid}" || argo_ec=$?
if [[ "${gitea_ec}" -ne 0 || "${argo_ec}" -ne 0 ]]; then
  echo "[gitops-lab] Parallel setup failed (gitea_ec=${gitea_ec} argo_ec=${argo_ec})" >&2
  exit 1
fi

bash scripts/map-cluster-dns.sh

# Repo-server clones via CoreDNS too — pin gitea hostname to pod IP (same CI DNS flake).
gitea_ip="$(kubectl -n gitops-lab get endpoints gitea -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
if [[ -n "${gitea_ip}" ]]; then
  echo "[gitops-lab] Pinning gitea.gitops-lab.svc.cluster.local → ${gitea_ip} on argocd-repo-server..."
  kubectl -n argocd patch deploy argocd-repo-server --type strategic --patch-file=/dev/stdin <<EOF
spec:
  template:
    spec:
      hostAliases:
        - ip: "${gitea_ip}"
          hostnames:
            - gitea.gitops-lab.svc.cluster.local
EOF
  kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=180s >/dev/null || true
fi

progress 75 "Registering Application"
echo "[gitops-lab] Registering repo + Application..."
kubectl apply -f manifests/application.yml

progress 80 "Configuring webhook"
echo "[gitops-lab] Configuring Gitea → Argo CD webhook..."
bash scripts/configure-webhook.sh

# Wait until Argo has synced once (Service selector bug => no endpoints is expected).
progress 85 "Waiting for Application sync"
echo "[gitops-lab] Waiting for Application demo-app to appear and sync..."
for i in $(seq 1 120); do
  sync=$(kubectl -n argocd get application demo-app -o jsonpath='{.status.sync.status}' 2>/dev/null || true)
  if [[ "${sync}" == "Synced" || "${sync}" == "OutOfSync" ]]; then
    break
  fi
  # Recover from transient repo-server dial failures — do NOT bounce CoreDNS (makes DNS worse).
  if (( i % 15 == 0 )); then
    cond="$(kubectl -n argocd get application demo-app -o jsonpath='{.status.conditions[0].message}' 2>/dev/null || true)"
    if [[ "${cond}" == *argocd-repo-server* || "${cond}" == *i/o\ timeout* || "${cond}" == *connection\ error* ]]; then
      echo "[gitops-lab] Application ComparisonError (${cond}); refreshing + bouncing application-controller..." >&2
      rs_addr="$(kubectl -n argocd get svc argocd-repo-server -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
      if [[ -n "${rs_addr}" && "${rs_addr}" != "None" ]]; then
        kubectl -n argocd patch configmap argocd-cmd-params-cm --type merge \
          -p "{\"data\":{\"repo.server\":\"${rs_addr}:8081\"}}" >/dev/null 2>&1 || true
      fi
      kubectl -n argocd delete pod -l app.kubernetes.io/name=argocd-application-controller --ignore-not-found >/dev/null 2>&1 || true
      sleep 8
    fi
    kubectl -n argocd annotate application demo-app argocd.argoproj.io/refresh=hard --overwrite >/dev/null 2>&1 || true
  fi
  sleep 2
done
kubectl -n argocd get application demo-app >/dev/null

echo "[gitops-lab] Waiting for demo-app Deployment from Argo sync..."
ok=0
for i in $(seq 1 120); do
  if ! kubectl -n gitops-lab get deploy demo-app >/dev/null 2>&1; then
    if (( i % 15 == 0 )); then
      kubectl -n argocd annotate application demo-app argocd.argoproj.io/refresh=hard --overwrite >/dev/null 2>&1 || true
    fi
    sleep 2
    continue
  fi
  ready="$(kubectl -n gitops-lab get deploy demo-app -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
  running="$(kubectl -n gitops-lab get pods -l app=demo-app --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')"
  if [[ "${ready:-0}" -ge 1 || "${running:-0}" -ge 1 ]]; then
    ok=1
    break
  fi
  sleep 2
done
if [[ "${ok}" != "1" ]]; then
  echo "[gitops-lab] demo-app never became Running after Argo sync" >&2
  kubectl -n argocd get application demo-app -o wide >&2 || true
  kubectl -n argocd get application demo-app -o yaml 2>&1 | tail -80 >&2 || true
  kubectl -n argocd get pods -o wide >&2 || true
  kubectl -n argocd get svc,endpoints argocd-repo-server -o wide >&2 || true
  kubectl -n kube-system get pods -l k8s-app=kube-dns -o wide >&2 || true
  kubectl -n gitops-lab get deploy,pods,svc -o wide >&2 || true
  exit 1
fi
kubectl -n gitops-lab rollout status deploy/demo-app --timeout=120s || true

progress 92 "Waiting for Argo CD UI"
echo "[gitops-lab] Waiting for Argo CD UI at /argocd..."
host="${K3SLAB_INGRESS_HOST}"
for _ in $(seq 1 20); do
  if curl -sf -H "Host: ${host}" "http://127.0.0.1/argocd/" >/dev/null 2>&1 \
    || curl -sf -H "Host: ${host}" "http://127.0.0.1/argocd" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

kubectl config set-context --current --namespace=gitops-lab >/dev/null

progress 98 "Finishing"
echo "[gitops-lab] Ready."
echo "[gitops-lab] Argo CD UI: http://${K3SLAB_INGRESS_HOST}/argocd/  (user=${STUDENT_ID} pass=${STUDENT_ID})"
echo "[gitops-lab] Gitea:      http://${K3SLAB_INGRESS_HOST}/gitea/"
echo "[gitops-lab] Git remote (in-cluster): http://<student>:<student>@gitea.gitops-lab.svc.cluster.local:3000/gitops/demo-app.git"
echo "[gitops-lab] Sync: Gitea push webhook → Argo CD (poll backup every 180s)"
