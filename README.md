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

If a workshop or your own manifests create **NodePort** Services or **Ingress** resources, the **terminal title bar** may show **Open in browser** tabs (the API watches the cluster over SSE). Links open on your **host** browser; there is no reverse proxy on port 3010.

| Lab | Ports to publish | URL (after fixes) |
|-----|------------------|-------------------|
| **kubectl Basics** (`01-kubectl-basics`) | `-p 3010:3010` | Workshop UI only |
| **Deployment Basics** (`02-deployment-basics`) | `-p 3010:3010` **and** `-p 80:80` | **`http://localhost/ctf/`** for the [simple-ctf](https://github.com/Exital/simple-ctf) app |

Example for the deployment lab:

```bash
docker run --rm --name k3slab \
  --privileged --cgroupns=host \
  -p 3010:3010 -p 80:80 \
  k3slab:latest
```

For other Ingress or NodePort workloads, publish the matching ports and set **`K3SLAB_PUBLIC_ORIGIN`** for NodePort URLs if needed.

#### Environment variables

- **`K3SLAB_PUBLIC_ORIGIN`** (optional): scheme + host for **NodePort** links. Default `http://localhost`.
- **`K3SLAB_INGRESS_HTTP_PORT`** / **`K3SLAB_INGRESS_HTTPS_PORT`** (optional): defaults **80** / **443** for Ingress URLs if you customized the ingress controller.
- **`K3SLAB_DISABLE_TRAEFIK`** (optional): default **`false`**. Set to **`true`** (or `1` / `yes`) to start K3s with `--disable=traefik`.
- **`k9s_enable`** (optional): default **`false`**. Set to **`true`** (or `1` / `yes`) to put the pre-installed **k9s** binary on `PATH`. The image bundles k9s at `/usr/local/lib/k3slab/k9s`; the entrypoint symlinks it to `/usr/local/bin/k9s` only when enabled.
- **`K3SLAB_DEBUG`** (optional): set to **`true`** (or `1` / `yes`) to log exposure watcher sync/resync messages (`exposure: synced …`, `exposure: periodic resync …`). Off by default so routine logs stay quiet.
- **`K3SLAB_ALLOW_CLUSTER_RESET`** (optional): default **`true`**. Set to **`false`** (or `0` / `no`) to disable **Restart lab** and **lab switching** (`POST /api/lab/restart`, `POST /api/labs/select`).
- **`LABS_ROOT`** (optional): parent directory of lab subfolders; default **`/lab`**.
- **`LAB_ID`** (optional): active lab folder name on startup (e.g. **`01-kubectl-basics`**); default **`01-kubectl-basics`** in the image.

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

### Custom labs mount

Labs live under **`LABS_ROOT`** (default **`/lab`**). Each **immediate subdirectory** with a `workshop.yml` is one lab (e.g. `lab/01-kubectl-basics`). The UI lists all labs and lets learners switch (switching resets the cluster).

#### Lab order in the picker and menu

The catalog is sorted **alphabetically by folder name** (the lab `id`). To control display order, prefix directory names with numbers. Shipped labs: **`01-kubectl-basics`**, **`02-deployment-basics`**.

Mount your own lab tree:

```bash
docker run --rm --name k3slab \
  --privileged \
  --cgroupns=host \
  -p 3010:3010 \
  -v "$(pwd)/lab:/lab" \
  k3slab:latest
```

A single custom lab: create `01-my-course/workshop.yml` and mount the parent directory to `/lab` (the lab id is the folder name, `01-my-course`). Optionally set **`-e LAB_ID=01-my-course`** to start on that lab without using the picker.

### Quick checks

- Health: `curl -sf http://127.0.0.1:3010/health`
- Lab status: `curl -s http://127.0.0.1:3010/api/lab/status` → `{"cluster":"ready"}`, `{"cluster":"resetting"}`, or `{"cluster":"unavailable"}`
- The UI starts before cluster readiness; if setup/verify actions stay disabled, watch `/api/lab/status` until it becomes `{"cluster":"ready"}`.

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
| [lab/01-kubectl-basics](lab/01-kubectl-basics) | **kubectl Basics** — intro `kubectl` questions |
| [lab/02-deployment-basics](lab/02-deployment-basics) | **Deployment Basics** — fix Deployment/Service/Ingress for [simple-ctf](https://github.com/Exital/simple-ctf) at `/ctf` |
| [app/frontend](app/frontend) | Vite + React + Tailwind + xterm.js |
| [lab](lab) | Baked-in labs tree (`01-kubectl-basics/`, `02-deployment-basics/`, …) |

## Writing workshops (`workshop.yml`)

The UI loads **`workshop.yml`** from the active lab directory under **`LABS_ROOT`** (default **`/lab/<lab-id>`**, shipped as **`01-kubectl-basics`**). The workshop defines a linear sequence of **`tabs.steps`** (tasks and questions) plus optional **`tabs.markdowns`** panels shown in the sidebar.

### Top-level fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Workshop title shown in the UI header. |
| `description` | No | Short blurb for the lab picker (not shown during steps). |
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
| `answer_type` | Yes | **`text`** (free-form input), **`single_choice`** (radio list), or **`observe`** (no input — UI polls cluster state). |
| `options` | If `single_choice` | Non-empty list of choice labels. The learner’s selection is passed to verify **verbatim** as `ANSWER` (the full string of the chosen option). |
| `poll_interval_seconds` | If `observe` | Integer **3–120**. How often the UI runs **`verify`** automatically (seconds). |
| `setup` | No | Commands to prepare the cluster **before** the learner can answer. Omit or use `[]` for no setup. |
| `verify` | Yes | Shell script for **`bash -lc`**. For **`text`** / **`single_choice`**, the learner’s submission is in **`ANSWER`**. For **`observe`**, only cluster/app checks (no **`ANSWER`**). Exit code **0** = correct (marks the question complete); non-zero = wrong or not ready yet. The learner advances with **Next question** in the UI after a correct verify. |
| `hints` | No | List of strings. The UI reveals them **one at a time** when the learner clicks “Show next hint”. |
| `incorrect_message` | No | Markdown shown when verify fails **instead of** the generic default wrong-answer panel. Omit to use the built-in copy. The panel stays until the learner dismisses it with **×**; if they submit another wrong answer after dismissing it, the panel appears again. |
| `correct_message` | No | Markdown shown after a correct verify. Optional; omit for no extra panel (the short “Correct!” banner still appears). The panel stays until dismissed with **×** or the learner clicks **Next question**. |

### `setup` shape

`setup` can be:

- **Omitted** or **`null`** — no setup commands.
- A **single string** — one command (equivalent to a one-element list).
- A **single object** — one command with explicit fields:
  - `run` (string, required): command to execute.
  - `background` (bool, optional): when `true`, start it and continue without waiting.
- A **list** containing strings and/or objects — processed in order; synchronous commands still fail-fast.

Each command is executed as **`bash -lc "<command>"`** with working directory set to the **active lab folder** (the subdirectory under **`LABS_ROOT`** that contains `workshop.yml`).

Example background setup entry:

```yaml
setup:
  - run: bash scripts/install-ingress.sh
    background: true
```

### Execution environment (for `run`, `setup`, and `verify`)

All workshop shell snippets run with:

- **Shell**: `bash -lc` (your YAML can use multi-line scripts, pipes, `kubectl`, `helm`, etc.).
- **Working directory**: the active lab directory so paths like `bash scripts/foo.sh` or `kubectl apply -f manifests/app.yml` resolve next to `workshop.yml`.
- **Environment**: Host environment plus **`KUBECONFIG`** (defaults to the in-container K3s kubeconfig) and **`HOME=/root`**. Submit verify also receives **`ANSWER`** set to the submitted string exactly as sent (for **`single_choice`**, that is always one of the **`options`** strings verbatim). **`observe`** checks do not set **`ANSWER`**.

The **browser terminal** starts in **`/root`** by default, or **`K3SLAB_TERMINAL_CWD`** after a lab is selected (set to that lab’s directory) so `kubectl apply -f manifests/...` paths match the workshop tree.

### Timeouts (engine defaults)

Rough guardrails: **task** and **question setup** up to about **10 minutes** each; **verify** up to about **5 minutes**. Very long-running scripts should be avoided in favor of quick checks.

### Learner flow and progression

- **Tasks** run automatically when their step becomes current; on success the next step loads immediately.
- **Questions** run **setup** automatically once per step (when the step becomes current and setup is not yet done). The learner then answers and submits (**`text`** / **`single_choice`**); **verify** runs on each submit until it exits 0.
- **`observe`** questions run **verify** on a timer after setup (no answer field). Failed polls are silent; success shows the same **Correct!** flow as a submitted answer.
- After a **correct** answer, the UI stays on the same question and shows **Next question** (replacing **Submit answer**). Optional **`correct_message`** / **`incorrect_message`** panels are dismissible with **×** and do not auto-hide like the brief success/failure banners at the top of the panel.
- **Wrong answer**: the incorrect panel (custom or default) stays visible across repeated wrong submits until dismissed. If the learner dismisses it and submits wrong again, the panel reappears.
- **Restart lab** in the UI resets workshop progress **and** cluster state (full K3s wipe). **Switching labs** from the header menu does the same cluster reset plus loads the other workshop.
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

**Observe question** (cluster state only; completes when verify exits 0):

```yaml
- id: q-ready
  type: question
  title: Wait for the app
  description: |
    Fix the Deployment until pods are ready — this step completes automatically.
  answer_type: observe
  poll_interval_seconds: 5
  correct_message: |
    The app is healthy.
  verify: |
    kubectl get deploy my-app -n my-ns -o jsonpath='{.status.readyReplicas}' | grep -q '^1$'
```

A full working file ships as [lab/01-kubectl-basics/workshop.yml](lab/01-kubectl-basics/workshop.yml). Mount your own tree over **`/lab`** (see [Custom labs mount](#custom-labs-mount)) to add or edit labs without rebuilding the image.

## API (same origin as UI)

- `GET /api/labs` — catalog of labs under `LABS_ROOT` (`labsRoot`, `activeId`, `labs[]` with `id`, `name`, `description`, `stepCount`, `valid`, `error`)  
- `POST /api/labs/select` — JSON `{ "id": "<lab-folder>" }`; activates a lab (full cluster reset when switching to a different lab); returns `{ "state": … }`  
- `GET /api/workshop` — current step and metadata (includes `sidebarTabs`, `labId`, `labsRoot`)  
- `GET /api/lab/status` — `{ "cluster": "ready" | "resetting" | "unavailable" }`  
- `POST /api/lab/restart` — reset cluster + workshop progress to step 1  
- `POST /api/workshop/restart` — reset workshop progress only (in-memory; internal/dev; no cluster wipe)  
- `POST /api/task/run` — run the current **task** step  
- `POST /api/question/setup` — run **setup** for the current question  
- `POST /api/question/submit` — JSON `{ "answer": "..." }` runs **verify** (`ANSWER` env); marks the question complete on success but does not advance  
- `POST /api/question/check` — runs **verify** for **`observe`** questions (no `ANSWER`); same response shape as submit  
- `POST /api/question/next` — advance to the next step after the current question was answered correctly  
- `GET /api/stream/logs` — SSE log stream for setup/task output  
- `GET /api/exposed` — JSON `{ "endpoints": [...] }` for NodePort / Ingress browser links  
- `GET /api/stream/exposed` — SSE stream of the same payload when Services or Ingresses change  
- `GET /api/ws/terminal` — WebSocket PTY (binary I/O + JSON `{"type":"resize","cols","rows"}` text frames)
