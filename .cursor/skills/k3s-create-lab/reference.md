# k3s-create-lab reference

Minimal skeletons. Prefer copying from `lab/01-kubectl-basics/` or `lab/02-deployment-basics/` for real examples.

## Minimal workshop.yml (task + text)

```yaml
name: My Lab
description: Short picker blurb.

tabs:
  steps:
    - id: prepare-lab
      type: task
      title: Prepare the lab cluster
      run: bash scripts/setup.sh

    - id: question-example
      type: question
      title: Example question
      description: |
        Ask the learner something checkable with kubectl.
      answer_type: text
      correct_message: |
        Correct.
      verify: |
        test "$ANSWER" = "expected"
      solution_answer: "expected"
      hints:
        - "Try `kubectl get …`"

  markdowns:
    - id: cheat-sheet
      title: Cheat sheet
      icon: description
      content: |
        # Tips
        - `kubectl get pods -A`
```

## Observe + background setup

```yaml
name: Troubleshoot Lab
cluster:
  disable_traefik: true

tabs:
  steps:
    - id: question-deploy
      type: question
      title: Deploy the app
      description: |
        Apply manifests under `manifests/`.
      answer_type: observe
      poll_interval_seconds: 5
      setup:
        - run: bash scripts/setup.sh
          background: true
      verify: |
        kubectl get ns my-lab &>/dev/null
        kubectl get deploy my-app -n my-lab &>/dev/null
      solution_script: |
        kubectl apply -f manifests/
```

## Minimal setup.sh

```bash
#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

progress() {
  local pct="$1"
  shift
  if command -v k3slab-progress >/dev/null 2>&1; then
    k3slab-progress "${pct}" "$@"
  else
    printf '::k3slab-progress::%s::%s\n' "${pct}" "$*"
  fi
}

progress 10 "Pre-pulling images"
# k3s ctr images pull docker.io/library/nginx:1.27-alpine

progress 50 "Applying lab manifests"
kubectl apply -f manifests/

progress 80 "Waiting for rollout"
# kubectl rollout status deployment/… -n … --timeout=120s

progress 100 "Ready"
```

## Field cheat sheet

| Area | Keys |
|------|------|
| Root | `name`, `description`, `cluster`, `tabs.steps`, `tabs.markdowns` |
| `cluster` | `disable_traefik`, `enable_network_policy` |
| task | `id`, `type`, `title`, `run` |
| question | `id`, `type`, `title`, `answer_type`, `verify`; optional `description`, `setup`, `hints`, messages, `options`, `poll_interval_seconds` |
| lab-test | `solution_answer`, `solution_script`, `solution_answer_script` |
| markdowns | `title`, `content`; optional `id`, `icon` |
