#!/usr/bin/env bash
set -euo pipefail

# K3s ships CoreDNS as a Deployment in kube-system.
kubectl -n kube-system scale deploy/coredns --replicas=0 >/dev/null
