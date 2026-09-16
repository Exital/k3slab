---
name: k3s-create-lab
description: >-
  Scaffold and author a new K3sLab workshop under lab/NN-slug with workshop.yml,
  setup scripts, manifests, and lab-test solution fields. Use when creating a
  new lab, adding a workshop, writing workshop.yml, or authoring graded questions.
---

# Create a K3sLab lab

## Workflow

1. Pick the next free folder: `lab/NN-slug/` (numeric prefix = catalog order; id = folder name).
2. Add required `workshop.yml`. Prefer the default layout below.
3. Add graded `solution_*` on every question that should pass `make test-lab`.
4. Smoke with `k3slab-run-test` (`make run` / `make test-lab`).

## Layout

```
lab/NN-slug/
  workshop.yml          # required — discovery key
  scripts/setup.sh      # default prep path
  manifests/            # kubectl/helm targets; optional *.yml.template
```

Discovery: only **immediate** children of `lab/` that contain `workshop.yml` are labs. Catalog sorts alphabetically by folder name.

## workshop.yml rules

- Prefer **`tabs.steps`** (+ optional **`tabs.markdowns`**). Do **not** mix with legacy top-level `steps`.
- Optional **`cluster`**: `disable_traefik`, `enable_network_policy` (see README / lab 02, 04).
- Every step needs a unique **`id`** and **`type`**: `task` or `question`.

### task

Only `id`, `type`, `title`, `run`. First step is usually:

```yaml
- id: prepare-lab
  type: task
  title: Prepare the lab cluster
  run: bash scripts/setup.sh
```

### question

- **`answer_type`**: `text` | `single_choice` | `observe`
- **`verify`**: `bash -lc`; exit 0 = pass. For `text` / `single_choice`, submission is in **`$ANSWER`**. `observe` has no `$ANSWER` (polls cluster).
- **`observe`**: set `poll_interval_seconds` (3–120). Often use background `setup` instead of a leading task (see lab 02).
- Optional: `description`, `hints`, `correct_message`, `incorrect_message`, `options` (required for `single_choice`).

### Lab-test fields (authors)

Required for CI coverage on that question; omitted questions are **skipped** by lab-test:

| Field | Use |
|-------|-----|
| `solution_answer` | Static answer → `$ANSWER` |
| `solution_script` | Fix cluster after setup; stdout **ignored** |
| `solution_answer_script` | Stdout becomes `$ANSWER` (dynamic) |

- `text` / `single_choice`: `solution_answer` and/or `solution_answer_script` (+ optional `solution_script`)
- `observe`: typically **`solution_script` only**; lab-test polls `verify`

Shell cwd for `run` / `setup` / `verify` / `solution_*` is the **lab directory**. Do not write under the lab tree in solutions (`make test-lab` mounts `lab/` **`:ro`**).

## setup.sh conventions

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

progress() {
  local pct="$1"; shift
  if command -v k3slab-progress >/dev/null 2>&1; then
    k3slab-progress "${pct}" "$@"
  else
    printf '::k3slab-progress::%s::%s\n' "${pct}" "$*"
  fi
}

# Pre-pull images with: k3s ctr images pull <image>
# Apply manifests, wait for rollouts, publish progress 0–100
```

## Manifests / Ingress

When Ingress `rules[].host` must follow env, use `manifests/*.yml.template` with `${K3SLAB_INGRESS_HOST}` **and** commit the rendered `*.yml` defaults. In verify/solutions, prefer `host="${K3SLAB_INGRESS_HOST:-localhost}"` for curl `Host` headers.

## Patterns to copy

| Pattern | Example |
|---------|---------|
| Task prep + text questions | `lab/01-kubectl-basics/` |
| Observe + background setup | `lab/02-deployment-basics/` |

## Source of truth

- README § **Writing workshops (`workshop.yml`)** and § **Optional lab test fields**
- Field types: `app/backend/workshop/types.go`
- Skeletons: [reference.md](reference.md)
