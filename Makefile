GITEA_PLATFORMS ?= linux/arm64,linux/amd64,linux/riscv64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7
MINIMAL_PLATFORM ?= linux/arm64,linux/amd64,linux/riscv64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7
ROOTLESS_PLATFORM ?= linux/amd64,linux/arm64

GITEA_TAG ?= ghcr.io/sirherobrine23/gitea:latest
RUNNER_TAG ?= ghcr.io/sirherobrine23/gitea_act

DOCKER_ARGS ?=

define gitea_build
	docker buildx build \
		$(DOCKER_ARGS) \
		-t $(GITEA_TAG) \
		--platform $(GITEA_PLATFORMS) \
		--target gitea .
endef

define runner_build_minimal
	docker buildx build \
		$(DOCKER_ARGS) \
		-t $(RUNNER_TAG):minimal \
		-t $(RUNNER_TAG):latest \
		--platform $(MINIMAL_PLATFORM) \
		--target minimal-runner .
endef

# Based in ghcr.io/catthehacker/ubuntu:act-*, limited archs
define runner_build_rootless
	docker buildx build \
		$(DOCKER_ARGS) \
		-t $(RUNNER_TAG):rootless-$(if $(filter latest,$(1)),latest,act-$(1)) \
		--platform $(ROOTLESS_PLATFORM) \
		--build-arg ACT_TAG=$(1) \
		--target rootless-runner .
endef

build: gitea runner

gitea:
	$(call gitea_build)

runner: runner-rootless runner-minimal runner-custom
runner-minimal:
	$(call runner_build_minimal)
runner-rootless:
	$(call runner_build_rootless,latest)
	$(call runner_build_rootless,24.04)
	$(call runner_build_rootless,22.04)
runner-custom:
	@echo Current not have custom docker image
