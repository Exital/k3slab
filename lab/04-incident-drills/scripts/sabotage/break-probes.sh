#!/usr/bin/env bash
set -euo pipefail

kubectl -n incident-lab patch deploy api --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/httpGet/path","value":"/readyz"},{"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/httpGet/path","value":"/livez"}]' >/dev/null
