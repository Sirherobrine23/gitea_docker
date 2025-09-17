# syntax=docker/dockerfile:1
# This image is to: linux/arm64,linux/amd64,linux/riscv64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7,linux/arm/v6
# Pull code
FROM --platform=$BUILDPLATFORM scratch AS pull
ARG GITEA_TAG="main" GITEA_REPO="https://github.com/go-gitea/gitea.git"
ADD --keep-git-dir=true ${GITEA_REPO}#${GITEA_TAG} /

# Base to build gitea
FROM --platform=$BUILDPLATFORM debian:sid AS debian_base
ARG DEBIAN_FRONTEND="noninteractive"
RUN <<EOF
set -e
apt update
apt install -y golang git wget curl make

# Install NodeJS
curl -fsSL https://deb.nodesource.com/setup_24.x | bash -
apt install -y nodejs
npm install -g pnpm@latest
EOF

# Copy go mod and node package and download
WORKDIR /build
COPY --from=pull /package.json /*-lock* /go.mod /go.sum ./
RUN pnpm install && go mod download

# Copy code
COPY --from=pull / ./
RUN --mount=type=bind,source=./,target=/tmp/build-context git apply /tmp/build-context/add_env.patch || git apply /tmp/build-context/old_add_env.patch
RUN make frontend

# Build frontend and prepare go build
FROM --platform=$BUILDPLATFORM debian_base AS backend
ARG TARGETOS TARGETARCH TARGETVARIANT GITEA_VERSION TAGS
RUN TAGS="bindata timetzdata $TAGS" GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=$TARGETVARIANT make backend

# Latest image
FROM debian:sid
ARG DEBIAN_FRONTEND="noninteractive"
RUN apt update && \
  apt install -y adduser && \
  userdel ubuntu; \
  addgroup --gid 1000 git; \
  useradd --uid 1000 --gid 1000 --groups git --home-dir /data -m --shell /bin/bash git && \
  apt remove --purge -y adduser && \
  apt install -y --no-install-suggests gnupg git && \
  rm -rf /var/lib/apt/ /var/cache/*

# Copy gitea
COPY --from=backend /build/gitea /usr/bin/gitea

# Setup gitea
ARG GITEA_VERSION=main
LABEL "br.com.sirherobrine23.gitea.hash"=${GITEA_VERSION}
USER git
VOLUME ["/data"]
WORKDIR /data
ENV GITEA_WORK_DIR="/data" GITEA_CUSTOM="/data"
ENTRYPOINT [ "gitea", "web", "--config", "/data/app.ini" ]
