#!/usr/bin/env bash
# Render lab manifests from templates using K3SLAB_* environment variables.
# Usage: render-lab-manifests.sh <lab-dir>
set -euo pipefail

lab_dir="${1:-}"
if [[ -z "${lab_dir}" || ! -d "${lab_dir}" ]]; then
  echo "usage: render-lab-manifests.sh <lab-dir>" >&2
  exit 1
fi

# Ingress hosts must be lowercase RFC 1123 (macOS .local names are often mixed-case).
export K3SLAB_INGRESS_HOST="$(printf '%s' "${K3SLAB_INGRESS_HOST:-localhost}" | tr '[:upper:]' '[:lower:]')"

if [[ -f "${lab_dir}/scripts/render-manifests.sh" ]]; then
  (cd "${lab_dir}" && bash scripts/render-manifests.sh) || true
  exit 0
fi

vars=$(env | awk -F= '/^K3SLAB_/ {printf "${%s} ", $1}')
shopt -s nullglob
for tpl in "${lab_dir}"/manifests/*.yml.template "${lab_dir}"/manifests/*.yaml.template; do
  out="${tpl%.template}"
  envsubst "$vars" < "$tpl" > "$out" 2>/dev/null || true
done
