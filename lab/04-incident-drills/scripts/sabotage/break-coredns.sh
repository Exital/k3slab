#!/usr/bin/env bash
set -euo pipefail

# Realistic incident: broken Corefile pushed by mistake.
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}' > /tmp/coredns-corefile.bak
cat > /tmp/coredns-corefile.bad <<'EOF'
.:53 {
    errors
    this_is_not_a_valid_directive
}
EOF
kubectl -n kube-system create configmap coredns --from-file=Corefile=/tmp/coredns-corefile.bad --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n kube-system rollout restart deploy/coredns >/dev/null
