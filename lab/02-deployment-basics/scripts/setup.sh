#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "[deployment-basics] Installing ingress-nginx for this lab..."
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx >/dev/null 2>&1 || true
helm repo update ingress-nginx >/dev/null 2>&1 || true
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace \
  --wait --timeout 10m \
  --set controller.replicaCount=1 \
  --set controller.hostNetwork=true \
  --set controller.dnsPolicy=ClusterFirstWithHostNet \
  --set controller.service.type=ClusterIP \
  --set controller.updateStrategy.type=Recreate \
  --set controller.ingressClassResource.name=nginx \
  --set controller.ingressClassResource.default=true \
  --set controller.admissionWebhooks.enabled=false \
  --set controller.watchIngressWithoutClass=true

echo "[deployment-basics] ingress-nginx is ready."
