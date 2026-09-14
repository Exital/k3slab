#!/usr/bin/env bash
# Lab 05 setup — fail-fast with stage timing (total budget ~8m).
set -euo pipefail

cd "$(dirname "$0")/.."

# Ingress hosts must be lowercase RFC 1123 (macOS .local names are often mixed-case).
export K3SLAB_INGRESS_HOST="$(printf '%s' "${K3SLAB_INGRESS_HOST:-localhost}" | tr '[:upper:]' '[:lower:]')"

# Overall prepare budget (seconds). CI must not sit for 15–25m.
SETUP_TIMEOUT_SEC="${K3SLAB_SETUP_TIMEOUT_SEC:-480}"
SETUP_START=$(date +%s)

raw="${student_username:-student}"
# shellcheck disable=SC2001
STUDENT_ID="$(printf '%s' "${raw}" | sed -E 's/[[:space:]]+/_/g')"
export STUDENT_ID

ARGOCD_CHART_VERSION="${ARGOCD_CHART_VERSION:-7.9.1}"
ARGOCD_APP_VERSION="${ARGOCD_APP_VERSION:-v2.14.11}"
GITEA_IMAGE="${GITEA_IMAGE:-docker.io/gitea/gitea:1.22.6}"
NGINX_IMAGE="${NGINX_IMAGE:-docker.io/library/nginx:1.27-alpine}"
REDIS_IMAGE="${REDIS_IMAGE:-public.ecr.aws/docker/library/redis:7.2.8-alpine}"
PYTHON_IMAGE="${PYTHON_IMAGE:-docker.io/library/python:3.12-alpine}"
ARGOCD_IMAGE="quay.io/argoproj/argocd:${ARGOCD_APP_VERSION}"

elapsed() { echo $(( $(date +%s) - SETUP_START )); }

stage() {
  echo "[gitops-lab] === STAGE: $* (t=$(elapsed)s) ==="
}

die() {
  echo "[gitops-lab] FATAL (t=$(elapsed)s): $*" >&2
  kubectl get ns 2>&1 | head -20 >&2 || true
  kubectl -n gitops-lab get pods,svc -o wide 2>&1 | head -40 >&2 || true
  kubectl -n argocd get pods,svc -o wide 2>&1 | head -40 >&2 || true
  exit 1
}

check_budget() {
  local left=$(( SETUP_TIMEOUT_SEC - $(elapsed) ))
  if [[ "${left}" -le 0 ]]; then
    die "overall setup timeout (${SETUP_TIMEOUT_SEC}s) exceeded"
  fi
  echo "[gitops-lab] budget remaining: ${left}s"
}

progress() {
  local pct="$1"
  shift
  if command -v k3slab-progress >/dev/null 2>&1; then
    k3slab-progress "${pct}" "$@"
  else
    printf '::k3slab-progress::%s::%s\n' "${pct}" "$*"
  fi
}

bcrypt_password() {
  local pass="$1"
  local hash=""
  hash="$(
    PASSWORD="${pass}" python3 - <<'PY' 2>/dev/null || true
import crypt, os, sys
pw = os.environ["PASSWORD"]
try:
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
  for _ in $(seq 1 60); do
    phase=$(kubectl -n argocd get pod bcrypt-gen -o jsonpath='{.status.phase}' 2>/dev/null || true)
    if [[ "${phase}" == "Succeeded" || "${phase}" == "Failed" ]]; then
      break
    fi
    sleep 1
  done
  hash="$(kubectl -n argocd logs bcrypt-gen 2>/dev/null | tr -d '\r' | grep -E '^\$2[aby]\$' | tail -n 1 || true)"
  kubectl -n argocd delete pod bcrypt-gen --ignore-not-found >/dev/null 2>&1 || true
  if [[ -z "${hash}" || "${hash}" != \$2* ]]; then
    return 1
  fi
  printf '%s\n' "${hash}"
}

apply_gitea() {
  stage "gitea: apply manifests"
  host_esc="${K3SLAB_INGRESS_HOST//\//\\/}"
  tmp_gitea="$(mktemp)"
  sed \
    -e "s|value: \"localhost\"|value: \"${K3SLAB_INGRESS_HOST}\"|" \
    -e "s|value: \"http://localhost/gitea/\"|value: \"http://${host_esc}/gitea/\"|" \
    manifests/gitea.yml >"${tmp_gitea}"
  kubectl apply -f "${tmp_gitea}"
  rm -f "${tmp_gitea}"

  stage "gitea: wait Traefik Middleware CRD"
  for _ in $(seq 1 45); do
    if kubectl get crd middlewares.traefik.io >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  kubectl wait --for=condition=Established crd/middlewares.traefik.io --timeout=60s >/dev/null
  kubectl apply -f manifests/gitea-stripprefix.yml
  tmp_gitea_ing="$(mktemp)"
  if [[ -f manifests/gitea-ingress.yml.template ]]; then
    export K3SLAB_INGRESS_HOST
    envsubst '${K3SLAB_INGRESS_HOST}' <manifests/gitea-ingress.yml.template >"${tmp_gitea_ing}"
  else
    sed "s/host: localhost/host: ${K3SLAB_INGRESS_HOST}/" manifests/gitea-ingress.yml >"${tmp_gitea_ing}"
  fi
  kubectl apply -f "${tmp_gitea_ing}"
  rm -f "${tmp_gitea_ing}"

  stage "gitea: rollout status (timeout 4m)"
  if ! kubectl -n gitops-lab rollout status deploy/gitea --timeout=240s; then
    kubectl -n gitops-lab get pods -l app=gitea -o wide >&2 || true
    kubectl -n gitops-lab logs -l app=gitea --tail=60 >&2 || true
    return 1
  fi
  kubectl -n gitops-lab wait --for=condition=Ready pod -l app=gitea --timeout=60s
  bash scripts/map-cluster-dns.sh
  stage "gitea: seed repo"
  bash scripts/seed-repo.sh
}

apply_webhook_proxy() {
  stage "webhook-proxy: apply + rollout (timeout 2m)"
  tmp_proxy="$(mktemp)"
  proxy_from="http://${K3SLAB_INGRESS_HOST}/gitea"
  proxy_from_esc="$(printf '%s' "${proxy_from}" | sed -e 's/[&|\\]/\\&/g')"
  sed -e "s|value: http://localhost/gitea|value: ${proxy_from_esc}|" \
    manifests/webhook-proxy.yml >"${tmp_proxy}"
  kubectl apply -f "${tmp_proxy}"
  rm -f "${tmp_proxy}"
  kubectl -n gitops-lab rollout status deploy/gitops-webhook-proxy --timeout=120s
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

  stage "argocd: helm install (timeout 6m)"
  helm upgrade --install argocd argo-cd \
    --repo https://argoproj.github.io/argo-helm \
    --namespace argocd \
    --version "${ARGOCD_CHART_VERSION}" \
    --values "${VALUES_FILE}" \
    --timeout 6m
  rm -f "${VALUES_FILE}"

  stage "argocd: wait core pods Running (timeout 3m)"
  ok=0
  for i in $(seq 1 90); do
    rs="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-repo-server --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    srv="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-server --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    ctrl="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-application-controller --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    redis="$(kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-redis --no-headers 2>/dev/null | awk '{print $3}' | head -n1 || true)"
    ep="$(kubectl -n argocd get endpoints argocd-repo-server -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
    if (( i % 10 == 0 )); then
      echo "[gitops-lab] argocd pods t=$(elapsed)s repo=${rs:-?} server=${srv:-?} ctrl=${ctrl:-?} redis=${redis:-?} ep=${ep:-none}"
    fi
    if [[ "${rs}" == "Running" && "${srv}" == "Running" && "${ctrl}" == "Running" && "${redis}" == "Running" && -n "${ep}" ]]; then
      ok=1
      break
    fi
    sleep 2
  done
  if [[ "${ok}" != "1" ]]; then
    kubectl -n argocd get pods -o wide >&2 || true
    return 1
  fi

  tmp_argocd_ing="$(mktemp)"
  if [[ -f manifests/argocd-ingress.yml.template ]]; then
    export K3SLAB_INGRESS_HOST
    envsubst '${K3SLAB_INGRESS_HOST}' <manifests/argocd-ingress.yml.template >"${tmp_argocd_ing}"
  else
    sed "s/host: localhost/host: ${K3SLAB_INGRESS_HOST}/" manifests/argocd-ingress.yml >"${tmp_argocd_ing}"
  fi
  kubectl apply -f "${tmp_argocd_ing}"
  rm -f "${tmp_argocd_ing}"

  kubectl -n argocd patch configmap argocd-cm --type merge \
    -p "{\"data\":{\"accounts.${STUDENT_ID}\":\"login\"}}" >/dev/null
  patch_json="$(jq -n --arg u "${STUDENT_ID}" --arg p "${bcrypt_hash}" \
    '{stringData: {("accounts." + $u + ".password"): $p}}')"
  kubectl -n argocd patch secret argocd-secret --type merge -p "${patch_json}" >/dev/null
  rbac_json="$(jq -n --arg u "${STUDENT_ID}" \
    '{data: {"policy.csv": ("g, " + $u + ", role:readonly\n"), "policy.default": "role:readonly"}}')"
  kubectl -n argocd patch configmap argocd-rbac-cm --type merge -p "${rbac_json}" >/dev/null
}

wire_argocd_pod_network() {
  local gitea_ip rs_ip
  gitea_ip="$(kubectl -n gitops-lab get endpoints gitea -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
  rs_ip="$(kubectl -n argocd get endpoints argocd-repo-server -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
  [[ -n "${gitea_ip}" && -n "${rs_ip}" ]] || return 1

  stage "wire: repo-server hostAlias gitea=${gitea_ip}"
  kubectl -n argocd patch deploy argocd-repo-server --type strategic --patch-file=/dev/stdin <<EOF
spec:
  template:
    spec:
      hostAliases:
        - ip: "${gitea_ip}"
          hostnames:
            - gitea.gitops-lab.svc.cluster.local
EOF
  kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=120s >/dev/null
  rs_ip="$(kubectl -n argocd get endpoints argocd-repo-server -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"

  stage "wire: pin repo.server=${rs_ip}:8081 + restart controller"
  kubectl -n argocd patch configmap argocd-cmd-params-cm --type merge \
    -p "{\"data\":{\"repo.server\":\"${rs_ip}:8081\"}}" >/dev/null
  if kubectl -n argocd get statefulset argocd-application-controller >/dev/null 2>&1; then
    kubectl -n argocd rollout restart statefulset/argocd-application-controller >/dev/null
    kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=120s >/dev/null
  fi

  # Webhook proxy → Argo / Gitea rewrite: use pod IPs (CoreDNS flaky on CI).
  argo_ip="$(kubectl -n argocd get endpoints argocd-server -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
  if [[ -n "${argo_ip}" ]]; then
    stage "wire: webhook-proxy env → argocd=${argo_ip} gitea=${gitea_ip}"
    kubectl -n gitops-lab set env deploy/gitops-webhook-proxy \
      "ARGO_WEBHOOK_URL=http://${argo_ip}/argocd/api/webhook" \
      "REPL_TO=http://${gitea_ip}:3000" >/dev/null
    kubectl -n gitops-lab rollout status deploy/gitops-webhook-proxy --timeout=90s >/dev/null
  fi
}

wait_application_sync() {
  local sync cond
  stage "sync: wait Application Synced/OutOfSync (timeout 3m)"
  for i in $(seq 1 90); do
    sync="$(kubectl -n argocd get application demo-app -o jsonpath='{.status.sync.status}' 2>/dev/null || true)"
    if [[ "${sync}" == "Synced" || "${sync}" == "OutOfSync" ]]; then
      echo "[gitops-lab] Application sync=${sync} (t=$(elapsed)s)"
      return 0
    fi
    if (( i % 15 == 0 )); then
      cond="$(kubectl -n argocd get application demo-app -o jsonpath='{.status.conditions[0].message}' 2>/dev/null || true)"
      echo "[gitops-lab] sync pending t=$(elapsed)s status=${sync:-?} cond=${cond:-none}"
      kubectl -n argocd annotate application demo-app argocd.argoproj.io/refresh=hard --overwrite >/dev/null 2>&1 || true
    fi
    sleep 2
  done
  return 1
}

wait_demo_app_running() {
  local ready running
  stage "sync: wait demo-app Running (timeout 3m)"
  for i in $(seq 1 90); do
    if kubectl -n gitops-lab get deploy demo-app >/dev/null 2>&1; then
      ready="$(kubectl -n gitops-lab get deploy demo-app -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
      running="$(kubectl -n gitops-lab get pods -l app=demo-app --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')"
      if [[ "${ready:-0}" -ge 1 || "${running:-0}" -ge 1 ]]; then
        echo "[gitops-lab] demo-app ready=${ready:-0} running=${running:-0} (t=$(elapsed)s)"
        return 0
      fi
    fi
    if (( i % 15 == 0 )); then
      echo "[gitops-lab] demo-app not ready yet t=$(elapsed)s"
    fi
    sleep 2
  done
  return 1
}

# --- main (under hard timeout) -----------------------------------------------

run_setup() {
  echo "[gitops-lab] Argo CD student identity: ${STUDENT_ID} (user=password)"
  echo "[gitops-lab] setup hard timeout: ${SETUP_TIMEOUT_SEC}s"

  progress 5 "Applying namespaces"
  stage "namespaces"
  kubectl apply -f manifests/00-namespace.yml
  for _ in $(seq 1 30); do
    if kubectl -n gitops-lab get sa default >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  kubectl -n gitops-lab get sa default >/dev/null
  check_budget

  progress 15 "Pre-pulling images"
  stage "pre-pull images (parallel)"
  pull_pids=()
  for img in "${GITEA_IMAGE}" "${NGINX_IMAGE}" "${REDIS_IMAGE}" "${ARGOCD_IMAGE}" "${PYTHON_IMAGE}"; do
    echo "[gitops-lab] pull start: ${img}"
    k3s ctr images pull "${img}" &
    pull_pids+=("$!")
  done
  pull_ec=0
  for pid in "${pull_pids[@]}"; do
    wait "${pid}" || pull_ec=1
  done
  [[ "${pull_ec}" -eq 0 ]] || die "image pull failed"
  echo "[gitops-lab] pre-pull done t=$(elapsed)s"
  check_budget

  # Note: do not wrap these in `cmd || die` — bash disables set -e inside
  # functions invoked from ||/&& lists, which silently skipped Ingress apply
  # failures (e.g. mixed-case macOS .local hostnames).
  progress 30 "Installing Gitea"
  apply_gitea
  check_budget

  progress 45 "Installing webhook proxy"
  apply_webhook_proxy
  check_budget

  progress 55 "Installing Argo CD"
  stage "bcrypt"
  BCRYPT_HASH="$(bcrypt_password "${STUDENT_ID}")" || die "bcrypt failed"
  install_argocd "${BCRYPT_HASH}"
  check_budget

  progress 65 "Wiring Argo pod network"
  wire_argocd_pod_network
  bash scripts/map-cluster-dns.sh
  check_budget

  progress 75 "Registering Application"
  stage "apply Application"
  kubectl apply -f manifests/application.yml

  progress 80 "Configuring webhook"
  stage "configure webhook"
  bash scripts/configure-webhook.sh
  check_budget

  progress 85 "Waiting for Application sync"
  wait_application_sync || die "Application never Synced/OutOfSync"
  wait_demo_app_running || die "demo-app never Running"
  kubectl -n gitops-lab rollout status deploy/demo-app --timeout=60s || true

  progress 92 "Waiting for Argo CD UI"
  stage "UI probe (non-blocking, 20s max)"
  host="${K3SLAB_INGRESS_HOST}"
  ui_ok=0
  for _ in $(seq 1 10); do
    if curl -sf --connect-timeout 2 --max-time 3 -H "Host: ${host}" "http://127.0.0.1/argocd/" >/dev/null 2>&1 \
      || curl -sf --connect-timeout 2 --max-time 3 -H "Host: ${host}" "http://127.0.0.1/argocd" >/dev/null 2>&1; then
      ui_ok=1
      break
    fi
    sleep 1
  done
  if [[ "${ui_ok}" -eq 1 ]]; then
    echo "[gitops-lab] Argo CD UI reachable (t=$(elapsed)s)"
  else
    echo "[gitops-lab] Argo CD UI not reachable yet (t=$(elapsed)s); continuing — lab still usable"
  fi

  kubectl config set-context --current --namespace=gitops-lab >/dev/null
  progress 98 "Finishing"
  stage "done total=$(elapsed)s"
  echo "[gitops-lab] Ready."
  echo "[gitops-lab] Argo CD UI: http://${K3SLAB_INGRESS_HOST}/argocd/  (user=${STUDENT_ID} pass=${STUDENT_ID})"
  echo "[gitops-lab] Gitea:      http://${K3SLAB_INGRESS_HOST}/gitea/"
}

# Hard kill if we exceed budget (GNU timeout in Ubuntu). Inner run skips re-wrapping.
if [[ "${K3SLAB_SETUP_INNER:-}" != "1" ]] && command -v timeout >/dev/null 2>&1; then
  export K3SLAB_SETUP_INNER=1
  export K3SLAB_SETUP_TIMEOUT_SEC="${SETUP_TIMEOUT_SEC}"
  set +e
  timeout --foreground --signal=TERM --kill-after=20s "${SETUP_TIMEOUT_SEC}s" bash "$0" "$@"
  ec=$?
  set -e
  if [[ "${ec}" -eq 124 ]] || [[ "${ec}" -eq 137 ]]; then
    echo "[gitops-lab] FATAL: setup killed by hard timeout (${SETUP_TIMEOUT_SEC}s) — last STAGE above is the hang point" >&2
    kubectl -n gitops-lab get pods -o wide 2>&1 | head -20 >&2 || true
    kubectl -n argocd get pods -o wide 2>&1 | head -20 >&2 || true
    exit 1
  fi
  exit "${ec}"
fi

run_setup
