#!/usr/bin/env bash
set -euo pipefail

node="$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')"
kubectl cordon "$node" >/dev/null
kubectl -n incident-lab scale deploy/web --replicas=3 >/dev/null
