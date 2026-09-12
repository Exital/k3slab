#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

progress() {
  local pct="$1"
  shift
  if command -v k3slab-progress >/dev/null 2>&1; then
    k3slab-progress "${pct}" "$@"
  else
    printf '::k3slab-progress::%s::%s\n' "${pct}" "$*"
  fi
}

METRICS_SERVER_IMAGE="${METRICS_SERVER_IMAGE:-registry.k8s.io/metrics-server/metrics-server:v0.9.0}"
NGINX_IMAGE="${NGINX_IMAGE:-docker.io/library/nginx:1.27-alpine}"
HPA_EXAMPLE_IMAGE="${HPA_EXAMPLE_IMAGE:-registry.k8s.io/hpa-example}"
PYTHON_IMAGE="${PYTHON_IMAGE:-docker.io/library/python:3.12-alpine}"
BUSYBOX_IMAGE="${BUSYBOX_IMAGE:-docker.io/library/busybox:1.36}"

progress 10 "Pre-pulling images"
echo "[workloads-lab] Pre-pulling images in parallel..."
pull_pids=()
for img in \
  "${METRICS_SERVER_IMAGE}" \
  "${NGINX_IMAGE}" \
  "${HPA_EXAMPLE_IMAGE}" \
  "${PYTHON_IMAGE}" \
  "${BUSYBOX_IMAGE}"; do
  k3s ctr images pull "${img}" &
  pull_pids+=("$!")
done
pull_ec=0
for pid in "${pull_pids[@]}"; do
  wait "${pid}" || pull_ec=1
done
if [[ "${pull_ec}" -ne 0 ]]; then
  echo "[workloads-lab] One or more image pulls failed" >&2
  exit 1
fi

progress 35 "Installing metrics-server"
# Ensure resource metrics exist for HPA and dashboard graphs (pinned + lab args baked in).
kubectl apply -f manifests/metrics-server.yml
kubectl -n kube-system rollout status deploy/metrics-server --timeout=180s
kubectl wait --for=condition=Available apiservice/v1beta1.metrics.k8s.io --timeout=180s

progress 55 "Applying lab manifests"
kubectl apply -f manifests/00-namespace.yml
kubectl apply -f manifests/rollout-app.yml
kubectl apply -f manifests/probe-app.yml
kubectl apply -f manifests/autoscale-app.yml

progress 70 "Waiting for deployments"
rollout_ec=0
kubectl -n workloads-lab rollout status deploy/web-rollout --timeout=120s &
web_pid=$!
kubectl -n workloads-lab rollout status deploy/autoscale-api --timeout=120s &
api_pid=$!
wait "${web_pid}" || rollout_ec=1
wait "${api_pid}" || rollout_ec=1
if [[ "${rollout_ec}" -ne 0 ]]; then
  echo "[workloads-lab] Deployment rollout failed" >&2
  exit 1
fi

progress 85 "Seeding challenge state"
# Seed rollout history with a bad release to force a real rollback challenge.
kubectl -n workloads-lab set image deploy/web-rollout web-rollout=nginx:not-a-real-tag >/dev/null

# Keep the autoscaling challenge deterministic.
kubectl -n workloads-lab delete hpa autoscale-api --ignore-not-found >/dev/null
kubectl -n workloads-lab delete pod loadgen --ignore-not-found >/dev/null
kubectl -n workloads-lab delete deploy loadgen --ignore-not-found >/dev/null
kubectl config set-context --current --namespace=workloads-lab >/dev/null

progress 95 "Finishing"
