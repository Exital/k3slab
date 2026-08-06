#!/usr/bin/env bash
set -euo pipefail

kubectl patch ingress shop-ingress -n incident-lab --type=json \
  -p '[{"op":"replace","path":"/spec/rules/0/http/paths/0/backend/service/name","value":"api"}]' >/dev/null
