#!/usr/bin/env bash
# Host-side orchestrator: light phases once, then one fresh container per lab.
# Used by `make test` (local). CI uses a matrix instead of this loop.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="${IMAGE:-k3slab-tests}"
REPORT_VOL="${REPORT_VOL:-k3slab-test-reports}"
LABS_ROOT="${ROOT}/lab"

discover_labs() {
  find "${LABS_ROOT}" -mindepth 2 -maxdepth 2 -name workshop.yml \
    | sed "s|^${LABS_ROOT}/||;s|/workshop.yml||" \
    | sort
}

run_container() {
  local name="$1"
  shift
  echo ""
  echo ">>> ${name}"
  docker run --rm --privileged --cgroupns=host \
    -v "${LABS_ROOT}:/src/lab:ro" \
    -e K3SLAB_TEST_REPORT_DIR=/reports \
    -e GITHUB_ACTIONS="${GITHUB_ACTIONS:-}" \
    -v "${REPORT_VOL}:/reports" \
    "$@" \
    "${IMAGE}"
}

main() {
  local failed=0
  local labs=()
  local lab

  while IFS= read -r lab; do
    [[ -n "${lab}" ]] || continue
    labs+=("${lab}")
  done < <(discover_labs)
  if [[ "${#labs[@]}" -eq 0 ]]; then
    echo "no labs found under ${LABS_ROOT}" >&2
    exit 1
  fi

  echo "Host tests: light phases, then ${#labs[@]} lab container(s)"

  if ! run_container "light phases" \
    -e K3SLAB_TEST_ONLY=backend-unit,backend-integration,frontend-build; then
    failed=1
  fi

  for lab in "${labs[@]}"; do
    if ! run_container "lab-e2e ${lab}" \
      -p 80:80 \
      -e K3SLAB_TEST_ONLY=lab-e2e \
      -e "K3SLAB_TEST_LAB=${lab}"; then
      failed=1
    fi
  done

  echo ""
  if [[ "${failed}" -ne 0 ]]; then
    echo "Host tests: FAIL"
    exit 1
  fi
  echo "Host tests: PASS"
}

main "$@"
