#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export K3SLAB_INGRESS_HOST="${K3SLAB_INGRESS_HOST:-localhost}"

INGRESS_NGINX_CHART_VERSION="${INGRESS_NGINX_CHART_VERSION:-4.12.1}"
# Must match chart appVersion / controller.image.tag for ${INGRESS_NGINX_CHART_VERSION}.
INGRESS_NGINX_IMAGE="${INGRESS_NGINX_IMAGE:-registry.k8s.io/ingress-nginx/controller:v1.12.1}"

echo "[deployment-basics] Pre-pulling ingress-nginx controller ${INGRESS_NGINX_IMAGE}..."
k3s ctr images pull "${INGRESS_NGINX_IMAGE}"

echo "[deployment-basics] Installing ingress-nginx (Helm ${INGRESS_NGINX_CHART_VERSION})..."
helm upgrade --install ingress-nginx ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx \
  --version "${INGRESS_NGINX_CHART_VERSION}" \
  --namespace ingress-nginx --create-namespace \
  --wait --timeout 10m \
  --set controller.replicaCount=1 \
  --set controller.hostNetwork=false \
  --set controller.hostPort.enabled=true \
  --set controller.hostPort.ports.http=80 \
  --set controller.service.type=ClusterIP \
  --set controller.service.enableHttps=false \
  --set controller.updateStrategy.type=Recreate \
  --set controller.admissionWebhooks.enabled=false \
  --set controller.ingressClassResource.name=nginx \
  --set controller.ingressClassResource.default=true \
  --set controller.watchIngressWithoutClass=true

# When lab manifests are read-only (e.g. make test-lab), patch the live Ingress host.
if [[ "${K3SLAB_INGRESS_HOST}" != "localhost" ]]; then
  for _ in $(seq 1 60); do
    if kubectl get ingress simple-ctf-ingress -n deployment-basics &>/dev/null; then
      current=$(kubectl get ingress simple-ctf-ingress -n deployment-basics -o jsonpath='{.spec.rules[0].host}' 2>/dev/null || true)
      if [[ "${current}" != "${K3SLAB_INGRESS_HOST}" ]]; then
        kubectl patch ingress simple-ctf-ingress -n deployment-basics --type=json \
          -p "[{\"op\":\"replace\",\"path\":\"/spec/rules/0/host\",\"value\":\"${K3SLAB_INGRESS_HOST}\"}]" \
          >/dev/null 2>&1 || true
      fi
      break
    fi
    sleep 2
  done
fi

echo "[deployment-basics] ingress-nginx is ready."
