# This image is to: linux/arm64,linux/amd64,linux/riscv64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7,linux/arm/v6
# Pull code
FROM --platform=$BUILDPLATFORM scratch AS pull
ARG GIT_HASH=main
ADD --keep-git-dir=true https://sirherobrine23.com.br/gitea/gitea.git#${GIT_HASH} /

# Build frontend
FROM --platform=$BUILDPLATFORM node:22 AS front
ARG DEBIAN_FRONTEND="noninteractive"
RUN apt update && apt install -y curl wget make

# Download packages
WORKDIR /build
COPY --from=pull /package.json /package-lock.json ./
RUN npm install

# Copy source and build frontend
COPY --from=pull / ./
RUN make frontend

# Build gitea final file
FROM --platform=$BUILDPLATFORM golang:latest AS backend

# Install tools to build gitea binary
ARG DEBIAN_FRONTEND="noninteractive"
RUN apt update && apt install -y curl wget make

# Copy go mod and download mods
WORKDIR /build
COPY --from=pull /go.mod /go.sum ./
RUN go mod download

# Copy source
COPY --from=front /build /build
ARG TARGETOS TARGETARCH TARGETVARIANT
RUN TAGS="bindata timetzdata" GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=$TARGETVARIANT make backend

# Latest image
FROM debian:sid

# Install basic tools to gitea
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
ARG GIT_HASH=main
LABEL "br.com.sirherobrine23.gitea.hash"=${GIT_HASH}
USER git
VOLUME ["/data"]
WORKDIR /data
ENV GITEA_WORK_DIR="/data" GITEA_CUSTOM="/data"
ENTRYPOINT [ "gitea", "web", "--config", "/data/app.ini" ]
