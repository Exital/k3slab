# K3sLab

Self-contained Kubernetes learning environment: **K3s**, **kubectl**, **Helm**, and a **browser UI** (terminal + guided workshop) in one image.

## Security

This image is meant for **trusted local learning only**. It exposes a real shell and cluster admin to anyone who can reach the web port.

## Build and run (Docker only)

All compilation, `npm install`, and `go mod tidy` happen **inside** `docker build`. You do **not** need Go, Node, or npm installed on your host to produce the image.

```bash
docker build -f docker/Dockerfile -t k3slab:latest .
```

The runtime image installs the **K3s binary** from GitHub releases (the `get.k3s.io` script expects systemd, which this image does not use). K3s is started by [docker/entrypoint.sh](docker/entrypoint.sh) when the container runs.

Run K3s and the UI. K3s in Docker needs **`--privileged`** and, on hosts that use **cgroup v2** (including Docker Desktop), **`--cgroupns=host`** so the kubelet can manage cgroups correctly:

```bash
docker run --rm --name k3slab \
  --privileged \
  --cgroupns=host \
  -p 3010:3010 \
  k3slab:latest
```

The entrypoint starts K3s with **`--snapshotter=native`** by default so containerd works when the container’s filesystem does not support nested overlay (common on Docker Desktop). You can override with `-e K3SLAB_K3S_SNAPSHOTTER=fuse-overlayfs` if you install and configure `fuse-overlayfs` in a custom image.

Open **`http://127.0.0.1:3010`** (recommended on Docker Desktop). If that works but `http://localhost:3010` does not, your machine is preferring IPv6 for `localhost`; see troubleshooting below.

### Custom lab mount

Mount your own workshop at `/lab/k3s` (must include `workshop.yml`):

```bash
docker run --rm --name k3slab \
  --privileged \
  --cgroupns=host \
  -p 3010:3010 \
  -v "$(pwd)/my-lab:/lab/k3s" \
  k3slab:latest
```

### Quick checks

- Health: `curl -sf http://127.0.0.1:3010/health`
- If the UI never loads, confirm the container logs show `Cluster is Ready` and `listening on 0.0.0.0:3010` (or your `K3SLAB_LISTEN` value).

### “Connection refused” on `http://localhost:3010`

1. **Use IPv4 explicitly**: some setups resolve `localhost` to `::1` while Docker only publishes IPv4 — open **`http://127.0.0.1:3010`** instead.
2. **Confirm the container is up**: `docker ps` should list `k3slab` and port `3010->3010`.
3. **Confirm K3s started**: `docker logs <container>` should end with `listening on …` without kubelet cgroup errors; if you see cgroup errors, add **`--cgroupns=host`** to `docker run`.
4. **Confirm nothing else owns the port**: change mapping to `-p 3011:3010` and open `http://127.0.0.1:3011`.

## Project layout

| Path | Role |
|------|------|
| [docker/Dockerfile](docker/Dockerfile) | Multi-stage image: Node build → Go build → Ubuntu runtime + K3s |
| [docker/entrypoint.sh](docker/entrypoint.sh) | Start K3s, wait for Ready, start `k3slab` API + static UI |
| [app/backend](app/backend) | Go API: workshop engine, SSE logs, PTY WebSocket terminal |
| [app/frontend](app/frontend) | Vite + React + Tailwind + xterm.js |
| [lab/k3s](lab/k3s) | Default baked-in workshop (`workshop.yml`, scripts, manifests) |

## Writing workshops (`workshop.yml`)

The UI loads **`workshop.yml`** from the lab directory (default **`/lab/k3s`** in the image, or your mount path). The file describes a linear sequence of **steps** learners complete in order.

### Top-level fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Workshop title shown in the UI header. |
| `steps` | Yes | Non-empty list of steps (see below). |

### Step types

Each step **must** have a unique string `id` and a `type` of either **`task`** or **`question`**.

#### `task` — automated setup

Runs once when the learner reaches this step, then the engine advances automatically. Use it for scripted cluster prep (apply manifests, run helper scripts, etc.).

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Stable identifier (used for debugging; not shown as a primary label). |
| `type` | Yes | Must be `task`. |
| `title` | Yes | Short label shown in the UI while the task runs. |
| `run` | Yes | Shell script passed to **`bash -lc`**. Exit code **0** marks success and moves to the next step; any other exit code fails the step and surfaces an error. |

Tasks do **not** support `description`, `setup`, or `verify` in the parser today—only `run`.

#### `question` — learner answer + verification

After setup finishes, the learner submits an answer; the **`verify`** script decides if it is correct.

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique step id. |
| `type` | Yes | Must be `question`. |
| `title` | Yes | Question heading. |
| `description` | No | Markdown body (bold, inline code, paragraphs, etc.) shown above the answer controls. |
| `answer_type` | Yes | Either **`text`** (free-form input) or **`single_choice`** (radio list). |
| `options` | If `single_choice` | Non-empty list of choice labels. The learner’s selection is passed to verify **verbatim** as `ANSWER` (the full string of the chosen option). |
| `setup` | No | Commands to prepare the cluster **before** the learner can answer. Omit or use `[]` for no setup. |
| `verify` | Yes | Shell script for **`bash -lc`**. The learner’s submission is in the environment variable **`ANSWER`**. Exit code **0** = correct (advance to next step); non-zero = wrong (stay on this question). |
| `hints` | No | List of strings. The UI reveals them **one at a time** when the learner clicks “Show next hint”. |
| `incorrect_message` | No | Markdown shown when verify fails **instead of** the generic default message. Omit to use the built-in copy. |

### `setup` shape

`setup` can be:

- **Omitted** or **`null`** — no setup commands.
- A **single string** — one command (equivalent to a one-element list).
- A **list of strings** — commands run **in order**; the first failure stops setup and leaves the question blocked until setup succeeds (e.g. after you fix the lab and refresh).

Each command is executed as **`bash -lc "<command>"`** with working directory **`LAB_ROOT`** (the lab folder containing `workshop.yml`).

### Execution environment (for `run`, `setup`, and `verify`)

All workshop shell snippets run with:

- **Shell**: `bash -lc` (your YAML can use multi-line scripts, pipes, `kubectl`, `helm`, etc.).
- **Working directory**: **`LAB_ROOT`** so paths like `bash scripts/foo.sh` or `kubectl apply -f manifests/app.yml` resolve next to `workshop.yml`.
- **Environment**: Host environment plus **`KUBECONFIG`** (defaults to the in-container K3s kubeconfig) and **`HOME=/root`**. Verify also receives **`ANSWER`** set to the submitted string exactly as sent (for **`single_choice`**, that is always one of the **`options`** strings verbatim).

The **browser terminal** starts in **`/root`** (not `LAB_ROOT`) so learners are not dropped straight into the lab tree; workshop commands still use `LAB_ROOT` as above.

### Timeouts (engine defaults)

Rough guardrails: **task** and **question setup** up to about **10 minutes** each; **verify** up to about **5 minutes**. Very long-running scripts should be avoided in favor of quick checks.

### Learner flow and progression

- **Tasks** run automatically when their step becomes current; on success the next step loads immediately.
- **Questions** run **setup** automatically once per step (when the step becomes current and setup is not yet done). The learner then answers and submits; **verify** runs on each submit until it exits 0.
- **Restart workshop** in the UI resets progress **in memory** to the first step (no cluster state rollback—design your labs or scripts accordingly).
- Progress is **per running app instance** (single shared learner if multiple browser tabs hit the same server).

### Minimal examples

**Task step:**

```yaml
- id: prep-cluster
  type: task
  title: Seed demo data
  run: bash scripts/setup.sh
```

**Text question with setup and hints:**

```yaml
- id: q-namespace
  type: question
  title: Which namespace is broken?
  description: |
    Use **kubectl** to find the pod in `CrashLoopBackOff` and submit the **namespace** name.
  answer_type: text
  setup:
    - kubectl apply -f manifests/broken-app.yml
  verify: |
    kubectl get pods -A | grep -q CrashLoopBackOff && test "$ANSWER" = "broken-demo"
  hints:
    - "Try `kubectl get pods -A`"
    - "Read the NAMESPACE column for that row"
```

**Single choice with custom wrong-answer copy:**

```yaml
- id: q-svc
  type: question
  title: Which command lists services?
  answer_type: single_choice
  options:
    - kubectl get pods
    - kubectl get svc
    - helm install
  incorrect_message: |
    Not quite. **Services** use the `Service` API — pick the `kubectl get` that matches.
  verify: 'test "$ANSWER" = "kubectl get svc"'
```

A full working file ships as [lab/k3s/workshop.yml](lab/k3s/workshop.yml). Mount your own directory over **`/lab/k3s`** (see [Custom lab mount](#custom-lab-mount)) to iterate on a workshop without rebuilding the image.

## API (same origin as UI)

- `GET /api/workshop` — current step and metadata  
- `POST /api/workshop/restart` — reset workshop progress to the beginning (in-memory)  
- `POST /api/task/run` — run the current **task** step  
- `POST /api/question/setup` — run **setup** for the current question  
- `POST /api/question/submit` — JSON `{ "answer": "..." }` runs **verify** (`ANSWER` env)  
- `GET /api/stream/logs` — SSE log stream for setup/task output  
- `GET /api/ws/terminal` — WebSocket PTY (binary I/O + JSON `{"type":"resize","cols","rows"}` text frames)
