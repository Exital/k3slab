#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure resource metrics exist for HPA and dashboard graphs.
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

kubectl apply -f manifests/00-namespace.yml
kubectl apply -f manifests/rollout-app.yml
kubectl apply -f manifests/probe-app.yml
kubectl apply -f manifests/autoscale-app.yml

kubectl -n workloads-lab rollout status deploy/web-rollout --timeout=120s
kubectl -n workloads-lab rollout status deploy/autoscale-api --timeout=120s

# Seed rollout history with a bad release to force a real rollback challenge.
kubectl -n workloads-lab set image deploy/web-rollout web-rollout=nginx:not-a-real-tag >/dev/null

# Keep the autoscaling challenge deterministic.
kubectl -n workloads-lab delete hpa autoscale-api --ignore-not-found >/dev/null
kubectl -n workloads-lab delete pod loadgen --ignore-not-found >/dev/null
kubectl -n workloads-lab delete deploy loadgen --ignore-not-found >/dev/null
kubectl config set-context --current --namespace=workloads-lab >/dev/null
