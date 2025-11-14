DOCKER_REGISTRY   ?= docker.io
IMAGE_PREFIX      ?= enricofoltran
SHORT_NAME        ?= simple-go-server

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

include versioning.mk
