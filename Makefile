DOCKER_REGISTRY   ?= docker.io
IMAGE_PREFIX      ?= enricofoltran
SHORT_NAME        ?= simple-go-server
IMAGE             := ${DOCKER_REGISTRY}/${IMAGE_PREFIX}/${SHORT_NAME}:latest

# build options
GO        ?= go
TAGS      :=
LDFLAGS   := -w -s
GOFLAGS   :=
BINDIR    := $(CURDIR)/bin

.PHONY: all
all: build

.PHONY: build
build:
	GOBIN=$(BINDIR) $(GO) install $(GOFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)'

.PHONY: clean
clean:
	@rm -rf $(BINDIR) coverage.out coverage.html

.PHONY: test
test:
	$(GO) test -v -race -coverprofile=coverage.out ./...

.PHONY: coverage
coverage: test
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: test-short
test-short:
	$(GO) test -v -short ./...

.PHONY: bench
bench:
	$(GO) test -v -bench=. -benchmem ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: lint
lint: vet fmt
	@echo "Linting complete"

.PHONY: check
check: lint test
	@echo "All checks passed"

.PHONY: run
run:
	$(GO) run main.go

.PHONY: check-docker
check-docker:
	@if [ -z $$(which docker) ]; then \
	  echo "Missing 'docker' client which is required for development"; \
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

.PHONY: docker-push
docker-push: docker-build
	docker push ${IMAGE}
