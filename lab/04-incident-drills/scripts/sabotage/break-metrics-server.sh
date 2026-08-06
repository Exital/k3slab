#!/usr/bin/env bash
set -euo pipefail

kubectl -n kube-system scale deploy/metrics-server --replicas=0 >/dev/null
