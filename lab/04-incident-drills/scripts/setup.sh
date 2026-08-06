#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# metrics-server for later infra drills (kubectl top).
kubectl apply -f "https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml"
ms_args="$(kubectl -n kube-system get deploy metrics-server -o jsonpath='{range .spec.template.spec.containers[0].args[*]}{.}{"\n"}{end}' 2>/dev/null || true)"
if ! printf '%s\n' "$ms_args" | grep -qx -- '--kubelet-insecure-tls'; then
  kubectl -n kube-system patch deploy metrics-server --type=json \
    -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]' >/dev/null
fi
if ! printf '%s\n' "$ms_args" | grep -qx -- '--kubelet-preferred-address-types=InternalIP,Hostname,InternalDNS,ExternalDNS,ExternalIP'; then
  kubectl -n kube-system patch deploy metrics-server --type=json \
    -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-preferred-address-types=InternalIP,Hostname,InternalDNS,ExternalDNS,ExternalIP"}]' >/dev/null
fi
kubectl -n kube-system rollout status deploy/metrics-server --timeout=180s
kubectl wait --for=condition=Available apiservice/v1beta1.metrics.k8s.io --timeout=180s

kubectl apply -f manifests/
kubectl -n incident-lab rollout status deploy/web --timeout=120s
kubectl -n incident-lab rollout status deploy/api --timeout=120s

bash scripts/sabotage/initial-breaks.sh

kubectl config set-context --current --namespace=incident-lab >/dev/null
