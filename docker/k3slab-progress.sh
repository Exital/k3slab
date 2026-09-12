#!/usr/bin/env bash
# Publish setup/task progress for the k3slab UI progress bar.
# Usage: k3slab-progress <0-100> [message...]
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: k3slab-progress <0-100> [message...]" >&2
  exit 1
fi

pct="$1"
shift

if ! [[ "${pct}" =~ ^[0-9]+$ ]] || (( pct < 0 || pct > 100 )); then
  echo "k3slab-progress: percent must be an integer 0-100, got: ${pct}" >&2
  exit 1
fi

msg="$*"
# Strip newlines so the marker stays a single line.
msg="${msg//$'\n'/ }"
msg="${msg//$'\r'/ }"

printf '::k3slab-progress::%s::%s\n' "${pct}" "${msg}"
