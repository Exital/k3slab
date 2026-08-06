#!/usr/bin/env bash
set -euo pipefail

# Seed the first incident wave: multiple independent breaks on the API stack.
kubectl -n incident-lab set image deploy/api api=nginx:not-a-real-tag >/dev/null
kubectl -n incident-lab patch configmap shop-api-config --type=merge -p '{"data":{"mode":"standby"}}' >/dev/null
kubectl -n incident-lab patch secret shop-api-secret --type=merge -p '{"stringData":{"token":"WRONG"}}' >/dev/null
kubectl -n incident-lab patch svc api-svc --type=json \
  -p '[{"op":"replace","path":"/spec/selector","value":{"app":"api-broken"}}]' >/dev/null
