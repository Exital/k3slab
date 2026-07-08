#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

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
