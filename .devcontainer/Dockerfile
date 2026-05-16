# PKS Fullstack DevContainer for pks-agent-tunnel
#
# Base contains: Node.js, .NET, Aspire CLI, Claude Code.
# We add Go for the server + CLI builds.

ARG BASE_IMAGE_TAG=latest

FROM ghcr.io/pksorensen/pks-fullstack-base:${BASE_IMAGE_TAG}

USER root

# Install Go
ARG GO_VERSION=1.24.2
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"

# Configure passwordless sudo for Docker-in-Docker (mirrors pks-agent-ftp).
RUN echo "node ALL=(root) NOPASSWD: /usr/local/share/docker-init.sh, /usr/bin/dockerd, /usr/bin/containerd, /usr/bin/pkill" > /etc/sudoers.d/node-docker && \
  chmod 0440 /etc/sudoers.d/node-docker

USER node
