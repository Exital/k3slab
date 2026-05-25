#!/usr/bin/env bash
set -euo pipefail

kubectl apply -f manifests/lab-env.yml

kubectl rollout status deployment/web -n kubectl-basics --timeout=120s
kubectl rollout status deployment/logger -n kubectl-basics --timeout=120s
