GIT_COMMIT = $(shell git rev-parse HEAD)
GIT_SHA    = $(shell git rev-parse --short HEAD)
GIT_TAG    = $(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
GIT_DIRTY  = $(shell test -n "`git status --porcelain`" && echo "dirty" || echo "clean")

ifdef VERSION
	DOCKER_VERSION = $(VERSION)
	BINARY_VERSION = $(VERSION)
endif

MUTABLE_VERSION := canary
DOCKER_VERSION  ?= git-${GIT_SHA}
BINARY_VERSION  ?= ${GIT_TAG}

# Only set Version if building a tag or VERSION is set
ifneq ($(BINARY_VERSION),)
	LDFLAGS += -X main.Version=${BINARY_VERSION}
endif

LDFLAGS += -X main.GitTag=${GIT_TAG}
LDFLAGS += -X main.GitCommit=${GIT_SHA}
LDFLAGS += -X main.GitTreeState=${GIT_DIRTY}

IMAGE         := ${DOCKER_REGISTRY}/${IMAGE_PREFIX}/${SHORT_NAME}:${DOCKER_VERSION}
MUTABLE_IMAGE := ${DOCKER_REGISTRY}/${IMAGE_PREFIX}/${SHORT_NAME}:${MUTABLE_VERSION}

info:
	@echo "Version:           ${VERSION}"
	@echo "Git Tag:           ${GIT_TAG}"
	@echo "Git Commit:        ${GIT_COMMIT}"
	@echo "Git Tree State:    ${GIT_DIRTY}"
	@echo "Docker Version:    ${DOCKER_VERSION}"
	@echo "Registry:          ${DOCKER_REGISTRY}"
	@echo "Immutable Image:   ${IMAGE}"
	@echo "Mutable Image:     ${MUTABLE_IMAGE}"

.PHONY: check-docker
check-docker:
	@if [ -z $$(which docker) ]; then \
	  echo "Missing \`docker\` client which is required for development"; \
	  exit 2; \
	fi

.PHONY: docker-binary
docker-binary: BINDIR = $(CURDIR)/rootfs
docker-binary: GOFLAGS += -a -installsuffix cgo
docker-binary:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(BINDIR)/$(SHORT_NAME) $(GOFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)'

.PHONY: docker-build
docker-build: check-docker docker-binary
	docker build --rm -t ${IMAGE} rootfs
	docker tag ${IMAGE} ${MUTABLE_IMAGE}

.PHONY: docker-push
docker-push: docker-mutable-push docker-immutable-push

.PHONY: docker-immutable-push
docker-immutable-push:
	docker push ${IMAGE}

.PHONY: docker-mutable-push
docker-mutable-push:
	docker push ${MUTABLE_IMAGE}