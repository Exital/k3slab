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

kubectl apply -f manifests/catalog-bot-rbac.yml
kubectl apply -f manifests/attacker-pod.yml

kubectl -n platform-lab wait --for=condition=Ready pod/attacker --timeout=120s

kubectl config set-context --current --namespace=platform-lab >/dev/null
