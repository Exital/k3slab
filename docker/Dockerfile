# --- Frontend (Vite) — dependencies install only inside this image layer ---
FROM node:22-bookworm AS frontend-build
WORKDIR /src/frontend
COPY app/frontend/package.json ./
RUN npm install --no-audit --no-fund
COPY app/frontend/ ./
RUN npm run build

# --- Backend (Go + CGO for PTY) ---
FROM golang:1.23-bookworm AS backend-build
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src/backend
COPY app/backend/ ./
COPY --from=frontend-build /src/frontend/dist ./dist
ENV CGO_ENABLED=1 GOOS=linux
RUN go mod tidy && go build -trimpath -ldflags="-s -w" -o /out/k3slab .

# --- Shared runtime base (prod + dev) ---
FROM ubuntu:24.04 AS k3s-base

ARG K3S_VERSION=v1.31.5+k3s1

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    jq \
    bash \
    python3 \
    iptables \
    procps \
    kmod \
    tar \
    vim \
    && rm -rf /var/lib/apt/lists/*

# K3s binary (install script expects systemd; use release asset instead)
RUN set -eux; \
    enc=$(printf '%s' "${K3S_VERSION}" | sed 's/+/%2B/g'); \
    arch="$(uname -m)"; \
    case "$arch" in \
      x86_64) sfx="" ;; \
      aarch64|arm64) sfx="-arm64" ;; \
      *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/k3s \
      "https://github.com/k3s-io/k3s/releases/download/${enc}/k3s${sfx}"; \
    chmod +x /usr/local/bin/k3s; \
    ln -sf /usr/local/bin/k3s /usr/local/bin/kubectl

RUN curl -sfL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

ARG K9S_VERSION=v0.50.18
RUN set -eux; \
    arch="$(uname -m)"; \
    case "$arch" in \
      x86_64) k9s_arch=Linux_amd64 ;; \
      aarch64|arm64) k9s_arch=Linux_arm64 ;; \
      *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
    esac; \
    mkdir -p /usr/local/lib/k3slab; \
    curl -fsSL -o /tmp/k9s.tar.gz \
      "https://github.com/derailed/k9s/releases/download/${K9S_VERSION}/k9s_${k9s_arch}.tar.gz"; \
    tar -xzf /tmp/k9s.tar.gz -C /usr/local/lib/k3slab k9s; \
    chmod +x /usr/local/lib/k3slab/k9s; \
    rm -f /tmp/k9s.tar.gz

WORKDIR /

COPY docker/k3s-lifecycle.sh /usr/local/lib/k3slab/k3s-lifecycle.sh
COPY docker/terminal-bashrc.sh /usr/local/lib/k3slab/terminal-bashrc.sh
COPY docker/profile.d-k3slab-terminal.sh /etc/profile.d/k3slab-terminal.sh
COPY docker/cluster-reset.sh /usr/local/bin/k3slab-cluster-reset
COPY docker/dev-entrypoint.sh /usr/local/bin/k3slab-dev-entrypoint
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /usr/local/lib/k3slab/k3s-lifecycle.sh /usr/local/bin/k3slab-cluster-reset /usr/local/bin/k3slab-dev-entrypoint /entrypoint.sh \
    && mkdir -p /run/k3slab /var/lib/k3slab \
    && touch /root/.bashrc \
    && ( grep -q 'terminal-bashrc.sh' /root/.bashrc 2>/dev/null || \
      { printf '%s\n' '# k3slab terminal prompt' '[ -f /usr/local/lib/k3slab/terminal-bashrc.sh ] && . /usr/local/lib/k3slab/terminal-bashrc.sh' '' | cat - /root/.bashrc > /tmp/k3slab-bashrc && mv /tmp/k3slab-bashrc /root/.bashrc; } )

# --- Dev runtime image: same base + hot-reload tooling ---
FROM k3s-base AS dev-runtime
ARG AIR_VERSION=v1.52.3
COPY --from=golang:1.23-bookworm /usr/local/go /usr/local/go
RUN ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
ENV PATH=/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/local/sbin:/usr/bin:/sbin:/bin
RUN GOBIN=/usr/local/bin /usr/local/go/bin/go install github.com/air-verse/air@${AIR_VERSION}
WORKDIR /workspace
ENV KUBECONFIG=/etc/rancher/k3s/k3s.yaml
ENV LABS_ROOT=/workspace/lab
ENV LAB_ID=01-kubectl-basics
ENV K3SLAB_LISTEN=0.0.0.0:3010
ENV K3SLAB_STATIC_DIR=/workspace/app/backend/dist
ENV K3SLAB_CLUSTER_PROFILE=/run/k3slab/cluster-profile.env
EXPOSE 3010
ENTRYPOINT ["/usr/local/bin/k3slab-dev-entrypoint"]

# --- CI / local test runner (privileged docker run; not published) ---
FROM k3s-base AS tests
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*
COPY --from=golang:1.23-bookworm /usr/local/go /usr/local/go
RUN ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
# Node/npm: copy runtime + global modules; rewrite npm/npx shims (symlinks break when cherry-picked).
COPY --from=node:22-bookworm /usr/local/bin/node /usr/local/bin/node
COPY --from=node:22-bookworm /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN printf '%s\n' '#!/bin/sh' 'exec node /usr/local/lib/node_modules/npm/bin/npm-cli.js "$@"' > /usr/local/bin/npm \
    && chmod +x /usr/local/bin/npm \
    && printf '%s\n' '#!/bin/sh' 'exec node /usr/local/lib/node_modules/npm/bin/npx-cli.js "$@"' > /usr/local/bin/npx \
    && chmod +x /usr/local/bin/npx
COPY --from=backend-build /out/k3slab /app/k3slab
COPY app/backend /src/backend
COPY app/frontend /src/frontend
COPY lab /src/lab
COPY docker/run-tests.sh /usr/local/bin/k3slab-run-tests
RUN chmod +x /usr/local/bin/k3slab-run-tests
ENV PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
ENV K3SLAB_BIN=/app/k3slab
ENV LABS_ROOT=/src/lab
ENV LAB_ID=01-kubectl-basics
ENV KUBECONFIG=/etc/rancher/k3s/k3s.yaml
ENV K3SLAB_K3S_SNAPSHOTTER=native
ENTRYPOINT ["/usr/local/bin/k3slab-run-tests"]

# --- Production runtime image (default target) ---
FROM k3s-base AS runtime
COPY --from=backend-build /out/k3slab /app/k3slab
COPY --from=frontend-build /src/frontend/dist /app/frontend/dist
COPY lab /lab
ENV KUBECONFIG=/etc/rancher/k3s/k3s.yaml
ENV LABS_ROOT=/lab
ENV LAB_ID=01-kubectl-basics
ENV K3SLAB_LISTEN=0.0.0.0:3010
ENV K3SLAB_STATIC_DIR=/app/frontend/dist
ENV k9s_enable=false
EXPOSE 3010
ENTRYPOINT ["/entrypoint.sh"]
