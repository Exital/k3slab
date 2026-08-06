#!/usr/bin/env bash
set -euo pipefail

# Realistic incident: accidental bad image rollout for metrics-server.
kubectl -n kube-system set image deploy/metrics-server \
  metrics-server=registry.k8s.io/metrics-server/metrics-server:not-a-real-tag >/dev/null
