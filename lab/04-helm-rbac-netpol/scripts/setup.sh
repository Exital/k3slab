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

# Pre-pull chart images so Helm install is not racing Docker Hub on cold CI runners.
# After earlier labs wipe cluster state, these pulls are the slow path on GitHub Actions.
echo "[platform-lab] Pre-pulling nginx and busybox images..."
k3s ctr images pull docker.io/library/nginx:1.27-alpine
k3s ctr images pull docker.io/library/busybox:1.36

kubectl apply -f manifests/catalog-bot-rbac.yml
kubectl apply -f manifests/attacker-pod.yml

kubectl -n platform-lab wait --for=condition=Ready pod/attacker --timeout=180s

kubectl config set-context --current --namespace=platform-lab >/dev/null
