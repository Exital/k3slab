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

# --- Runtime ---
FROM ubuntu:24.04

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
COPY docker/cluster-reset.sh /usr/local/bin/k3slab-cluster-reset
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /usr/local/lib/k3slab/k3s-lifecycle.sh /usr/local/bin/k3slab-cluster-reset /entrypoint.sh \
    && mkdir -p /run/k3slab /var/lib/k3slab

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
