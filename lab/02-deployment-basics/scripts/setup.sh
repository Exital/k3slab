#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "[deployment-basics] Waiting for API server..."
for _ in $(seq 1 60); do
  if kubectl get nodes --no-headers 2>/dev/null | grep -q Ready; then
    break
  fi
  sleep 2
done

echo "[deployment-basics] Waiting for ingress-nginx..."
for _ in $(seq 1 90); do
  if kubectl get deploy -n ingress-nginx ingress-nginx-controller &>/dev/null; then
    break
  fi
  sleep 2
done

kubectl apply -f manifests/

echo "[deployment-basics] Lab manifests applied in namespace deployment-basics."
echo "Use kubectl to diagnose and fix the Deployment, Service, and Ingress, then open http://localhost/ctf/ (publish -p 80:80 on docker run)."
