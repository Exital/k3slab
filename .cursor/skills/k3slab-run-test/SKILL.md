---
name: k3slab-run-test
description: >-
  Run and test K3sLab locally with Make/Docker: privileged container flags,
  ports, LAB_ID and ingress env vars, make run, make test-lab, and make test.
  Use when running a lab, debugging Docker startup, or verifying a workshop
  with lab-e2e tests.
---

# Run and test K3sLab

Canonical commands live in the repo root `Makefile`. Prefer Make over ad-hoc `docker` unless customizing flags.

## Run (interactive)

```bash
make run LAB=<lab-id>
# e.g. make run LAB=01-kubectl-basics
```

`make run` builds then starts Docker with:

- `--privileged` and `--cgroupns=host` (required for K3s in Docker / cgroup v2)
- UI: **`-p 3010:3010`** → open `http://127.0.0.1:3010`
- Also **`-p 80:80`** (Makefile always maps 80 — needed for Ingress labs)

| Lab | Needs host port 80 |
|-----|--------------------|
| `01-kubectl-basics` | No (UI only) |
| `02-deployment-basics` | Yes (`/ctf/`) |
| `03-workloads-probes-scaling` | Yes |
| `04-helm-rbac-netpol` | No |
| `05-gitops-argocd` | Yes (Argo CD / Gitea) |

### Useful env (Make / `docker run -e`)

| Variable | Role |
|----------|------|
| `LAB` / `LAB_ID` | Active lab folder name |
| `K3SLAB_INGRESS_HOST` | Ingress hostname (**hostname**, not IP); default `localhost` |
| `K3SLAB_PUBLIC_ORIGIN` | Origin for NodePort links; default `http://localhost` |
| `student_username` | Lab 05 Argo CD user/password (spaces → `_`) |
| `k9s_enable` | `true` to put k9s on `PATH` |

On a VM accessed remotely, set `K3SLAB_INGRESS_HOST` to the machine hostname and open `http://<hostname>/…`.

## Test

```bash
# One lab e2e (recommended while authoring)
make test-lab LAB=01-kubectl-basics

# Full suite (light phases + each lab sequentially)
make test

# Unit/integration/frontend only (no lab e2e)
make test-light
```

Notes:

- Test image mounts **`lab/` read-only** (`:ro`). Solution/setup scripts must not write into the lab tree.
- Questions without `solution_answer` / `solution_script` / `solution_answer_script` are **skipped** by lab-test.
- After authoring a new lab: smoke with `make run LAB=…`, then `make test-lab LAB=…`.

## Authoring loop

1. Edit under `lab/<id>/` (see `k3s-create-lab`).
2. `make run LAB=<id>` — exercise UI / kubectl manually.
3. `make test-lab LAB=<id>` — confirm graded steps pass.
4. Fix verify/solutions until green.

## Source of truth

- `Makefile`
- README: Build and run, env vars, Optional lab test fields
