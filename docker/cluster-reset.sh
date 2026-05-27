#!/usr/bin/env bash
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export PATH="/usr/local/bin:/usr/local/sbin:/usr/bin:/sbin:/bin:${PATH}"

# shellcheck source=/dev/null
source /usr/local/lib/k3slab/k3s-lifecycle.sh

k3slab_load_cluster_profile
reset_cluster
