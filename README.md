# K3sLab

Self-contained Kubernetes learning environment: **K3s**, **kubectl**, **Helm**, and a **browser UI** (terminal + guided workshop) in one image.

## Security

This image is meant for **trusted local learning only**. It exposes a real shell and cluster admin to anyone who can reach the web port. **Restart lab** in the UI wipes all cluster state (`/var/lib/rancher/k3s`) and workshop progress — use only in local lab environments.

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

### Opening apps (NodePort / Ingress)

When you expose workloads with **NodePort** Services or **Ingress** resources, the **terminal title bar** shows **Open in browser** tabs. They appear and disappear automatically as you create or delete Services and Ingresses (the API watches the cluster and pushes updates over SSE).

Links open on your **host** browser. There is no reverse proxy on port 3010 — you must publish the same ports on `docker run -p` that the workload uses inside the container.

| Exposure | Container port | Typical `docker run` |
|----------|----------------|----------------------|
| Workshop UI | 3010 | `-p 3010:3010` |
| Ingress (K3s Traefik, HTTP) | **80** | `-p 80:80` |
| Ingress (HTTPS / TLS) | **443** | `-p 443:443` |
| NodePort | **30000–32767** (per Service) | `-p <nodePort>:<nodePort>` (match `kubectl get svc`) |

**Build and run** (rebuild the image after pulling code changes):

```bash
docker build -f docker/Dockerfile -t k3slab:test .

docker run --rm --name k3slab \
  --privileged \
  --cgroupns=host \
  -p 3010:3010 \
  -p 80:80 \
  -p 443:443 \
  -p 30010:30010 \
  -e K3SLAB_PUBLIC_ORIGIN=http://127.0.0.1 \
  k3slab:test
```

**Sample manifests** (from the web terminal, paths are under `/lab/k3s`):

| File | What it does | Tab label (example) | Open in browser |
|------|----------------|---------------------|-----------------|
| [lab/k3s/manifests/demo-ingress.yml](lab/k3s/manifests/demo-ingress.yml) | nginx + Ingress on `localhost` / `/my_app` | `localhost/my_app` | `http://localhost/my_app/` |
| [lab/k3s/manifests/demo-nodeport.yml](lab/k3s/manifests/demo-nodeport.yml) | nginx + NodePort **30010** | `web:30010` | `http://127.0.0.1:30010/` (with origin above) |

```bash
kubectl apply -f manifests/demo-ingress.yml
kubectl apply -f manifests/demo-nodeport.yml
```

Ingress rules must use a **DNS hostname** (e.g. `localhost`), not an IP — Kubernetes rejects IP addresses in `spec.rules[].host`.

#### Tab labels and URLs

The UI shows the backend **`label`**; the click opens **`url`** (also shown on hover).

| Kind | Tab label | URL |
|------|-----------|-----|
| **NodePort** | `{serviceName}:{nodePort}` | `{K3SLAB_PUBLIC_ORIGIN host}:{nodePort}/` |
| **Ingress (HTTP)** | `{host}` or `{host}{path}` if path is not `/` | `http://{host}{path}` (port 80 omitted) |
| **Ingress (HTTPS)** | same as HTTP + ` (https)` | `https://{host}{path}` when TLS covers that host |

`K3SLAB_PUBLIC_ORIGIN` affects **NodePort URLs only**. Ingress tabs always use the **hostname from the Ingress rule** (not the public-origin host).

#### Environment variables

- **`K3SLAB_PUBLIC_ORIGIN`** (optional): scheme + host for **NodePort** links. Default `http://localhost`. Use `http://127.0.0.1` if `localhost` resolves to IPv6 (`::1`) but Docker published IPv4 only.
- **`K3SLAB_INGRESS_HTTP_PORT`** / **`K3SLAB_INGRESS_HTTPS_PORT`** (optional): defaults **80** / **443** for Ingress URLs if you customized Traefik.
- **`k9s_enable`** (optional): default **`false`**. Set to **`true`** (or `1` / `yes`) to put the pre-installed **k9s** binary on `PATH`. The image bundles k9s at `/usr/local/lib/k3slab/k9s`; the entrypoint symlinks it to `/usr/local/bin/k9s` only when enabled.
- **`K3SLAB_DEBUG`** (optional): set to **`true`** (or `1` / `yes`) to log exposure watcher sync/resync messages (`exposure: synced …`, `exposure: periodic resync …`). Off by default so routine logs stay quiet.
- **`K3SLAB_ALLOW_CLUSTER_RESET`** (optional): default **`true`**. Set to **`false`** (or `0` / `no`) to disable **Restart lab** (`POST /api/lab/restart`).

#### Troubleshooting tabs

1. **Rebuild the image** after updating k3slab — `docker build` then `docker run` (Docker does not pull `k3slab:test` from a registry; a missing local image fails with “pull access denied”).
2. **Publish ports** — e.g. `-p 80:80` for Ingress, `-p 30010:30010` for the demo NodePort.
3. **Check the API** — `curl -s http://127.0.0.1:3010/api/exposed` should list `endpoints` (not `null`) after you apply a NodePort Service or Ingress.
4. **Ingress host** — open the URL with the rule’s host (e.g. `http://localhost/my_app/`, not `http://127.0.0.1/my_app/` unless the rule uses that host).

#### Runtime tools in the image

The container includes **kubectl**, **helm**, **jq**, **git**, **vim**, and a bash shell (see [docker/Dockerfile](docker/Dockerfile)). **k9s** is bundled but off by default; enable at run time with `-e k9s_enable=true`:

```bash
docker run --rm --name k3slab \
  --privileged --cgroupns=host \
  -p 3010:3010 \
  -e k9s_enable=true \
  k3slab:latest
```

Pin the bundled k9s version at build time: `docker build --build-arg K9S_VERSION=v0.50.18 -f docker/Dockerfile -t k3slab:latest .` (default `v0.50.18`). Supported platforms: **linux/amd64** and **linux/arm64** (same as the K3s binary).

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
- Lab status: `curl -s http://127.0.0.1:3010/api/lab/status` → `{"cluster":"ready"}`, `{"cluster":"resetting"}`, or `{"cluster":"unavailable"}`
- If the UI never loads, confirm the container logs show `Cluster is Ready` and `listening on 0.0.0.0:3010` (or your `K3SLAB_LISTEN` value).

### Restart lab

The **Restart lab** button in the UI (header, `sm` screens and up) performs a full reset:

1. Stops the in-container K3s server
2. Deletes cluster data at `/var/lib/rancher/k3s`
3. Starts K3s again and waits until the node is **Ready**
4. Resets workshop progress to step 1 (the first task runs automatically)

Typical duration is **~20–60 seconds**; the API times out after **5 minutes** if K3s does not become Ready. During reset, **kubectl** in the terminal may fail briefly and cluster-dependent API calls return **409**. If reset fails, the UI shows an error and asks you to **stop and recreate the container**; the reset script attempts to restore K3s when possible.

This is distinct from **`POST /api/workshop/restart`**, which resets workshop progress **in memory only** and does not touch cluster state (internal/dev use).

If reset fails, restart the container (`docker run …` again) for a clean slate.

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
| [docker/k3s-lifecycle.sh](docker/k3s-lifecycle.sh) | Shared K3s start/stop/wait/reset helpers (entrypoint + API reset) |
| [docker/cluster-reset.sh](docker/cluster-reset.sh) | Cluster wipe script invoked by `POST /api/lab/restart` |
| [app/backend](app/backend) | Go API: workshop engine, exposure watcher, SSE logs, PTY WebSocket terminal |
| [lab/k3s/manifests](lab/k3s/manifests) | Sample manifests (workshop + demo Ingress / NodePort) |
| [app/frontend](app/frontend) | Vite + React + Tailwind + xterm.js |
| [lab/k3s](lab/k3s) | Default baked-in workshop (`workshop.yml`, scripts, manifests) |

## Writing workshops (`workshop.yml`)

The UI loads **`workshop.yml`** from the lab directory (default **`/lab/k3s`** in the image, or your mount path). The workshop defines a linear sequence of **`tabs.steps`** (tasks and questions) plus optional **`tabs.markdowns`** panels shown in the sidebar.

### Top-level fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Workshop title shown in the UI header. |
| `tabs` | Yes* | Object with `steps` (progression) and optional `markdowns` (sidebar). |
| `steps` | Yes* | **Legacy only:** top-level list of task/question steps if `tabs` is not used. Do not combine with `tabs.steps`. |

\*Provide either **`tabs.steps`** (preferred) or legacy **`steps`**, non-empty.

### `tabs` shape

```yaml
name: My workshop

tabs:
  steps:
    - id: my-task
      type: task
      title: ...
      run: ...
    - id: my-question
      type: question
      ...
  markdowns:
    - id: cheat-sheet          # optional; generated from title if omitted
      title: Cheat sheet
      icon: description         # optional Material Symbols name (e.g. info, menu_book)
      content: |
        # Markdown
        ...
```

- **`tabs.steps`**: ordered list of **`task`** and **`question`** entries (same rules as the step types below).
- **`tabs.markdowns`**: optional ordered list of reference panels. Each entry needs **`title`** and **`content`** (Markdown). Optional **`id`** (stable key for the UI). Optional **`icon`**: a [Material Symbols](https://fonts.google.com/icons) icon id (e.g. `description`, `info`); if omitted, the UI uses a default icon.

### Step types

Each step in **`tabs.steps`** (or legacy **`steps`**) **must** have a unique string `id` and a `type` of either **`task`** or **`question`**.

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
| `verify` | Yes | Shell script for **`bash -lc`**. The learner’s submission is in the environment variable **`ANSWER`**. Exit code **0** = correct (marks the question complete); non-zero = wrong (stay on this question). The learner advances with **Next question** in the UI after a correct verify. |
| `hints` | No | List of strings. The UI reveals them **one at a time** when the learner clicks “Show next hint”. |
| `incorrect_message` | No | Markdown shown when verify fails **instead of** the generic default wrong-answer panel. Omit to use the built-in copy. The panel stays until the learner dismisses it with **×**; if they submit another wrong answer after dismissing it, the panel appears again. |
| `correct_message` | No | Markdown shown after a correct verify. Optional; omit for no extra panel (the short “Correct!” banner still appears). The panel stays until dismissed with **×** or the learner clicks **Next question**. |

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
- After a **correct** answer, the UI stays on the same question and shows **Next question** (replacing **Submit answer**). Optional **`correct_message`** / **`incorrect_message`** panels are dismissible with **×** and do not auto-hide like the brief success/failure banners at the top of the panel.
- **Wrong answer**: the incorrect panel (custom or default) stays visible across repeated wrong submits until dismissed. If the learner dismisses it and submits wrong again, the panel reappears.
- **Restart lab** in the UI resets workshop progress **and** cluster state (full K3s wipe). Use it when the learner wants a clean slate; expect ~20–60 seconds while the cluster restarts.
- Progress is **per running app instance** (single shared learner if multiple browser tabs hit the same server).

### Minimal examples

Full file structure with **`tabs`**:

```yaml
name: Example

tabs:
  steps:
    - id: prep-cluster
      type: task
      title: Seed demo data
      run: bash scripts/setup.sh
  markdowns:
    - title: Tips
      icon: menu_book
      content: |
        Try `kubectl get nodes`.
```

**Task step** (inside `tabs.steps` or legacy `steps`):

```yaml
- id: prep-cluster
  type: task
  title: Seed demo data
  run: bash scripts/setup.sh
```

**Text question** (inside `tabs.steps` or legacy `steps`):

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

**Single choice** with custom feedback messages:

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
  correct_message: |
    Right — `kubectl get svc` lists **Service** objects in the cluster.
  verify: 'test "$ANSWER" = "kubectl get svc"'
```

A full working file ships as [lab/k3s/workshop.yml](lab/k3s/workshop.yml). Mount your own directory over **`/lab/k3s`** (see [Custom lab mount](#custom-lab-mount)) to iterate on a workshop without rebuilding the image.

## API (same origin as UI)

- `GET /api/workshop` — current step and metadata (includes `sidebarTabs` from `tabs.markdowns`)  
- `GET /api/lab/status` — `{ "cluster": "ready" | "resetting" | "unavailable" }`  
- `POST /api/lab/restart` — reset cluster + workshop progress to step 1  
- `POST /api/workshop/restart` — reset workshop progress only (in-memory; internal/dev; no cluster wipe)  
- `POST /api/task/run` — run the current **task** step  
- `POST /api/question/setup` — run **setup** for the current question  
- `POST /api/question/submit` — JSON `{ "answer": "..." }` runs **verify** (`ANSWER` env); marks the question complete on success but does not advance  
- `POST /api/question/next` — advance to the next step after the current question was answered correctly  
- `GET /api/stream/logs` — SSE log stream for setup/task output  
- `GET /api/exposed` — JSON `{ "endpoints": [...] }` for NodePort / Ingress browser links  
- `GET /api/stream/exposed` — SSE stream of the same payload when Services or Ingresses change  
- `GET /api/ws/terminal` — WebSocket PTY (binary I/O + JSON `{"type":"resize","cols","rows"}` text frames)
