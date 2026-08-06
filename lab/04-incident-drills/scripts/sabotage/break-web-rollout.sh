#!/usr/bin/env bash
set -euo pipefail

kubectl -n incident-lab set image deploy/web web=nginx:not-a-real-tag >/dev/null
