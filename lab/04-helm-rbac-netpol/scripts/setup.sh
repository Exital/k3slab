#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

kubectl apply -f manifests/00-namespace.yml

# Namespace controller creates the default SA asynchronously.
for _ in $(seq 1 60); do
  if kubectl -n platform-lab get sa default >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
kubectl -n platform-lab get sa default >/dev/null

# Pre-pull chart/job images so Helm install and RBAC job are not racing registries on cold CI.
echo "[platform-lab] Pre-pulling nginx, busybox, and bitnami/kubectl in parallel..."
pull_pids=()
for img in \
  docker.io/library/nginx:1.27-alpine \
  docker.io/library/busybox:1.36 \
  docker.io/bitnami/kubectl:1.31; do
  k3s ctr images pull "${img}" &
  pull_pids+=("$!")
done
pull_ec=0
for pid in "${pull_pids[@]}"; do
  wait "${pid}" || pull_ec=1
done
if [[ "${pull_ec}" -ne 0 ]]; then
  echo "[platform-lab] One or more image pulls failed" >&2
  exit 1
fi

kubectl apply -f manifests/catalog-bot-rbac.yml
kubectl apply -f manifests/attacker-pod.yml

kubectl -n platform-lab wait --for=condition=Ready pod/attacker --timeout=180s

kubectl config set-context --current --namespace=platform-lab >/dev/null
