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

echo "[deployment-basics] Waiting for Traefik Middleware CRD..."
for _ in $(seq 1 90); do
  if kubectl get crd middlewares.traefik.io &>/dev/null || kubectl get crd middlewares.traefik.containo.us &>/dev/null; then
    break
  fi
  sleep 2
done

kubectl apply -f manifests/namespace.yml
kubectl apply -f manifests/entrypoint-configmap.yml

if kubectl get crd middlewares.traefik.io &>/dev/null; then
  MID_API=traefik.io/v1alpha1
elif kubectl get crd middlewares.traefik.containo.us &>/dev/null; then
  MID_API=traefik.containo.us/v1alpha1
else
  echo "ERROR: Traefik Middleware CRD not found (expected middlewares.traefik.io or middlewares.traefik.containo.us)" >&2
  exit 1
fi

sed "s|apiVersion: traefik.io/v1alpha1|apiVersion: ${MID_API}|" manifests/middleware-strip-ctf.yml | kubectl apply -f -

kubectl apply -f manifests/deployment.yml
kubectl apply -f manifests/service.yml
kubectl apply -f manifests/ingress.yml

echo "[deployment-basics] Lab manifests applied in namespace deployment-basics."
echo "Use kubectl to diagnose and fix the Deployment, Service, and Ingress, then open http://localhost/ctf/ (publish -p 80:80 on docker run)."
