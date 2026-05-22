# syntax=docker/dockerfile:1

# This image is to: linux/arm64,linux/amd64,linux/riscv64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7
# debian dropped linux/arm/v6 maintence

# Flags global
ARG ACT_TAG=latest

# Pull codes
FROM --platform=$BUILDPLATFORM scratch AS gitea_code
ARG GITEA_TAG="main"
ADD --keep-git-dir=true https://github.com/go-gitea/gitea.git#${GITEA_TAG} /

FROM --platform=$BUILDPLATFORM scratch AS runner_code
ADD --keep-git-dir=true https://gitea.com/gitea/act_runner.git#main /

## Base system to build
FROM --platform=$BUILDPLATFORM debian:sid AS base_sys
ARG DEBIAN_FRONTEND="noninteractive"
RUN <<EOF
set -e
apt update
apt install -y golang git wget curl make ca-certificates nodejs
update-ca-certificates
corepack install -g npm
corepack install -g pnpm
corepack enable npm
corepack enable pnpm
npm install -g corepack --force
pnpm -v
npm -v
EOF

### Gitea Runner
FROM base_sys AS gitea_runner_base
WORKDIR /build
COPY --from=runner_code /go.mod /go.sum ./
RUN go mod download
COPY --from=runner_code / ./
RUN --mount=type=bind,source=./patches/act/,target=/tmp/build-context \
    find /tmp/build-context -type f | xargs -i{} git apply --ignore-whitespace "{}"
RUN go mod download
# Build latest gitea runner file
FROM --platform=$BUILDPLATFORM gitea_runner_base AS runner
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=$TARGETVARIANT make

### Gitea
FROM base_sys AS gitea_builder_base
# Copy go mod and node package and download
WORKDIR /build
COPY --from=gitea_code /go.mod /go.sum ./
RUN go mod download
# COPY --from=gitea_code /package.json /pnpm-lock.yaml /pnpm-workspace.yaml ./
COPY --from=gitea_code /package.json /pnpm-*.yaml ./
RUN pnpm install --no-frozen-lockfile
# Copy code
COPY --from=gitea_code / ./
RUN --mount=type=bind,source=./patches/gitea/,target=/tmp/build-context \
  (git apply /tmp/build-context/add_env.patch || git apply /tmp/build-context/old_add_env.patch); \
  find /tmp/build-context -type f | grep -v add_env.patch | xargs -i{} git apply --ignore-whitespace "{}"
RUN make frontend
# Build frontend and prepare go build
FROM --platform=$BUILDPLATFORM gitea_builder_base AS gitea_backend
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG GITEA_VERSION
ARG GITEA_TAGS
RUN TAGS="bindata timetzdata $GITEA_TAGS" GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=$TARGETVARIANT make backend

# Gitea latest image
FROM debian:stable AS gitea
LABEL org.opencontainers.image.source="https://sirherobrine23.com.br/Sirherobrine23/gitea_docker.git"
LABEL org.opencontainers.image.description="Gitea image"
LABEL org.opencontainers.image.licenses=MIT
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
COPY --from=gitea_backend /build/gitea /usr/bin/gitea
COPY --from=runner /build/gitea-runner /usr/bin/gitea-runner

# Setup gitea
ARG GITEA_VERSION=main
LABEL "br.com.sirherobrine23.gitea.hash"=${GITEA_VERSION}
USER git
VOLUME ["/data"]
WORKDIR /data
ENV GITEA_WORK_DIR="/data" GITEA_CUSTOM="/data"
ENTRYPOINT [ "gitea", "web", "--config", "/data/app.ini" ]

## Runner minimal - docker dind
FROM debian:sid AS minimal-runner
LABEL org.opencontainers.image.source="https://sirherobrine23.com.br/Sirherobrine23/act_runner_dind.git"
LABEL org.opencontainers.image.description="Gitea gitea runner image only with docker basic"
LABEL org.opencontainers.image.licenses=MIT
ARG TARGETARCH DEBIAN_FRONTEND="noninteractive"
RUN <<EOF
set -eux
# Docker
apt update
apt install -y docker-compose docker-cli docker-buildx iproute2 fuse-overlayfs slirp4netns

# Remove cache files
rm -rf /var/cache/apt/
EOF
COPY --from=runner /build/gitea-runner /usr/bin/gitea-runner
COPY ./runner.sh /usr/bin/run.sh
RUN chmod a+x /usr/bin/run.sh && mkdir /data
ENTRYPOINT [ "/usr/bin/run.sh" ]


##### ********************* rootless ********************* #####
# This images run host target without system root privileges
# for example railway.com

# rootless
FROM ghcr.io/catthehacker/ubuntu:act-${ACT_TAG} AS rootless-runner
LABEL org.opencontainers.image.source="https://sirherobrine23.com.br/Sirherobrine23/act_runner_dind.git"
LABEL org.opencontainers.image.description="Gitea rootless gitea runner image"
LABEL org.opencontainers.image.licenses=MIT
ARG TARGETARCH DEBIAN_FRONTEND="noninteractive"
RUN <<EOF
set -eux

# Basic Openwrt packages
pkgs="sudo openssh-client build-essential clang flex bison g++ gawk gettext git libncurses5-dev libssl-dev python3-setuptools rsync swig unzip zlib1g-dev file wget ccache golang"
case $TARGETARCH in
  x86_64|amd64|386)
		pkgs="$pkgs gcc-multilib g++-multilib"
		;;
  arm64|aarch64)
		echo No needs libs
		;;
esac

# Docker
pkgs="$pkgs docker.io docker-compose docker-cli iproute2 fuse-overlayfs slirp4netns"

apt update
apt install -y $pkgs

# Remove cache files
rm -rf /var/cache/apt/
EOF
COPY ./sudoers /etc/sudoers
RUN grep ubuntu -q /etc/passwd || (useradd -m -s /bin/bash ubuntu && \
    usermod -aG root,sudo,users ubuntu) && \
    ((getent group docker || groupadd -g 975 docker) && \
    usermod -aG docker ubuntu); \
    chmod 440 /etc/sudoers && chown root:root /etc/sudoers; \
    mkdir /workspace /data && chown ubuntu:ubuntu -R /workspace /data && chmod 7777 -R /workspace /data
COPY --from=runner /build/gitea-runner /usr/bin/gitea-runner
COPY ./runner.sh /usr/bin/run.sh
RUN chmod a+x /usr/bin/run.sh
WORKDIR /workspace
ENV GITEA_RUNNER_LABELS=ubuntu-${ACT_TAG}:host
USER ubuntu
ENTRYPOINT [ "/usr/bin/run.sh" ]
