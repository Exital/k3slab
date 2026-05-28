#!/usr/bin/env bash
# Unified test runner for the tests Docker image (local + GitHub Actions).
set -euo pipefail

export PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:${PATH}"
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export CGO_ENABLED="${CGO_ENABLED:-1}"
export K3SLAB_INTEGRATION_LABS_ROOT="${K3SLAB_INTEGRATION_LABS_ROOT:-/src/lab}"

: "${K3SLAB_TEST_FAIL_FAST:=1}"
: "${K3SLAB_TEST_ONLY:=}"
: "${K3SLAB_TEST_LAB:=}"
: "${K3SLAB_TEST_REPORT_DIR:=}"
: "${K3SLAB_BIN:=/app/k3slab}"
: "${LABS_ROOT:=/src/lab}"
: "${K3SLAB_K3S_SNAPSHOTTER:=native}"

# shellcheck source=/dev/null
source /usr/local/lib/k3slab/k3s-lifecycle.sh

SRC_BACKEND=/src/backend
SRC_FRONTEND=/src/frontend

declare -a PHASE_NAMES=()
declare -a PHASE_STATUS=()
declare -a PHASE_DURATION=()
declare -a PHASE_MESSAGES=()
OVERALL_FAILED=0
K3SLAB_SKIP_REMAINING=0

should_run_phase() {
  local name="$1"
  if [[ -z "${K3SLAB_TEST_ONLY}" ]]; then
    return 0
  fi
  [[ "${K3SLAB_TEST_ONLY}" == "${name}" ]]
}

run_phase() {
  local name="$1"
  shift
  if [[ "${K3SLAB_SKIP_REMAINING}" == "1" ]]; then
    return 0
  fi
  if ! should_run_phase "${name}"; then
    return 0
  fi

  echo ""
  echo "=== [${name}] ==="
  local log_file=""
  if [[ -n "${K3SLAB_TEST_REPORT_DIR}" ]]; then
    mkdir -p "${K3SLAB_TEST_REPORT_DIR}/logs"
    log_file="${K3SLAB_TEST_REPORT_DIR}/logs/${name}.log"
  fi

  local start end code
  start=$(date +%s)
  set +e
  if [[ -n "${log_file}" ]]; then
    "$@" > >(tee "${log_file}") 2>&1
    code=$?
  else
    "$@"
    code=$?
  fi
  set -e
  end=$(date +%s)
  local dur=$((end - start))

  PHASE_NAMES+=("${name}")
  PHASE_DURATION+=("${dur}")

  if [[ "${code}" -eq 0 ]]; then
    PHASE_STATUS+=("PASS")
    PHASE_MESSAGES+=("")
    echo "=== [${name}] PASS (${dur}s) ==="
  else
    PHASE_STATUS+=("FAIL")
    PHASE_MESSAGES+=("exit ${code}")
    OVERALL_FAILED=1
    echo "=== [${name}] FAIL (${dur}s) exit ${code} ==="
    if [[ "${K3SLAB_TEST_FAIL_FAST}" == "1" ]]; then
      K3SLAB_SKIP_REMAINING=1
    fi
  fi
  return 0
}

run_backend_unit() {
  (cd "${SRC_BACKEND}" && go test ./...)
}

run_backend_integration() {
  (cd "${SRC_BACKEND}" && go test -tags=integration ./integration/...)
}

run_frontend_build() {
  (cd "${SRC_FRONTEND}" && npm ci --no-audit --no-fund && npm run build)
}

run_lab_e2e() {
  local profile_lab="${K3SLAB_TEST_LAB:-${LAB_ID:-01-kubectl-basics}}"
  echo "[lab-e2e] Applying cluster profile for ${profile_lab}..."
  "${K3SLAB_BIN}" apply-cluster-profile "${LABS_ROOT}" "${profile_lab}"

  echo "[lab-e2e] Starting K3s..."
  K3S_PID=$(start_k3s)
  export K3S_PID
  if ! wait_ready; then
    echo "[lab-e2e] K3s failed to become ready"
    stop_k3s
    return 1
  fi
  local lab_args=(lab-test --labs-root "${LABS_ROOT}" --json)
  if [[ -n "${K3SLAB_TEST_LAB}" ]]; then
    lab_args+=(--lab "${K3SLAB_TEST_LAB}")
  fi

  local report_base="${K3SLAB_TEST_REPORT_DIR:-/tmp/k3slab-reports}"
  mkdir -p "${report_base}"
  local json_out="${report_base}/lab-e2e.json"

  set +e
  # JSON on stdout (tee'd to reports); live progress on stderr.
  "${K3SLAB_BIN}" "${lab_args[@]}" | tee "${json_out}"
  local code=${PIPESTATUS[0]}
  set -e

  stop_k3s
  return "${code}"
}

write_reports() {
  [[ -n "${K3SLAB_TEST_REPORT_DIR}" ]] || return 0
  mkdir -p "${K3SLAB_TEST_REPORT_DIR}"

  local md="${K3SLAB_TEST_REPORT_DIR}/summary.md"
  local js="${K3SLAB_TEST_REPORT_DIR}/summary.json"

  {
    echo "# k3slab test summary"
    echo ""
    echo "| Phase | Status | Duration |"
    echo "|-------|--------|----------|"
    local i total=0
    for i in "${!PHASE_NAMES[@]}"; do
      local dur="${PHASE_DURATION[$i]:-0}"
      total=$((total + dur))
      echo "| ${PHASE_NAMES[$i]} | ${PHASE_STATUS[$i]} | ${dur}s |"
    done
    echo ""
    if [[ "${OVERALL_FAILED}" -eq 0 ]]; then
      echo "**TOTAL: PASS** (${total}s)"
    else
      echo "**TOTAL: FAIL** (${total}s)"
      echo ""
      for i in "${!PHASE_NAMES[@]}"; do
        if [[ "${PHASE_STATUS[$i]}" == "FAIL" ]]; then
          echo "- **${PHASE_NAMES[$i]}**: ${PHASE_MESSAGES[$i]:-failed}"
        fi
      done
    fi
  } > "${md}"

  {
    echo '{'
    echo '"phases":['
    local i first=1
    for i in "${!PHASE_NAMES[@]}"; do
      [[ "${first}" -eq 1 ]] || echo ','
      first=0
      printf '{"name":"%s","status":"%s","duration_s":%s,"message":"%s"}' \
        "${PHASE_NAMES[$i]}" "${PHASE_STATUS[$i]}" "${PHASE_DURATION[$i]:-0}" "${PHASE_MESSAGES[$i]:-}"
    done
    echo '],'
    if [[ "${OVERALL_FAILED}" -eq 0 ]]; then
      echo '"total":"PASS"'
    else
      echo '"total":"FAIL"'
    fi
    echo '}'
  } > "${js}"

  if [[ -n "${GITHUB_STEP_SUMMARY:-}" && -f "${md}" && -d "$(dirname "${GITHUB_STEP_SUMMARY}")" ]]; then
    cat "${md}" >> "${GITHUB_STEP_SUMMARY}"
  fi
}

print_summary() {
  local total=0 i
  echo ""
  echo "══════════════════════════════════════════════════"
  echo " k3slab test summary"
  echo "══════════════════════════════════════════════════"
  printf " %-22s %-8s %s\n" "Phase" "Status" "Duration"
  echo "──────────────────────────────────────────────────"
  for i in "${!PHASE_NAMES[@]}"; do
    local dur="${PHASE_DURATION[$i]:-0}"
    total=$((total + dur))
    printf " %-22s %-8s %ss\n" "${PHASE_NAMES[$i]}" "${PHASE_STATUS[$i]}" "${dur}"
    if [[ "${PHASE_STATUS[$i]}" == "FAIL" && -n "${PHASE_MESSAGES[$i]:-}" ]]; then
      echo "   -> ${PHASE_MESSAGES[$i]}"
    fi
  done
  echo "──────────────────────────────────────────────────"
  if [[ "${OVERALL_FAILED}" -eq 0 ]]; then
    printf " %-22s %-8s %ss\n" "TOTAL" "PASS" "${total}"
  else
    printf " %-22s %-8s %ss\n" "TOTAL" "FAIL" "${total}"
    echo ""
    echo " Failures:"
    for i in "${!PHASE_NAMES[@]}"; do
      if [[ "${PHASE_STATUS[$i]}" == "FAIL" ]]; then
        echo "   ${PHASE_NAMES[$i]}: ${PHASE_MESSAGES[$i]:-failed}"
        if [[ -n "${K3SLAB_TEST_REPORT_DIR}" ]]; then
          echo "     log: ${K3SLAB_TEST_REPORT_DIR}/logs/${PHASE_NAMES[$i]}.log"
        fi
      fi
    done
  fi
  echo "══════════════════════════════════════════════════"

  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    for i in "${!PHASE_NAMES[@]}"; do
      if [[ "${PHASE_STATUS[$i]}" == "FAIL" ]]; then
        echo "::error title=${PHASE_NAMES[$i]}::${PHASE_MESSAGES[$i]:-failed}"
      fi
    done
  fi
}

main() {
  run_phase backend-unit run_backend_unit
  run_phase backend-integration run_backend_integration
  run_phase frontend-build run_frontend_build
  run_phase lab-e2e run_lab_e2e

  write_reports
  print_summary

  if [[ "${OVERALL_FAILED}" -ne 0 ]]; then
    exit 1
  fi
}

main "$@"
