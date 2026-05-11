.PHONY: all clean znnd docs doc-lint doc-api

GO ?= latest

ifeq ($(OS),Windows_NT) 
    detected_OS := Windows
else
    detected_OS := $(shell sh -c 'uname 2>/dev/null || echo Unknown')
endif

ifeq ($(detected_OS),Windows)
    EXECUTABLE=libznn.dll
endif
ifeq ($(detected_OS),Darwin)
    EXECUTABLE=libznn.dylib
endif
ifeq ($(detected_OS),Linux)
    EXECUTABLE=libznn.so
endif

SERVERMAIN = $(shell pwd)/cmd/znnd
LIBMAIN = $(shell pwd)/cmd/libznn
BUILDDIR = $(shell pwd)/build
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_COMMIT_FILE=$(shell pwd)/metadata/git_commit.go

$(EXECUTABLE):
	go build -o $(BUILDDIR)/$(EXECUTABLE) -buildmode=c-shared -tags libznn $(LIBMAIN)

libznn: $(EXECUTABLE) ## Build binaries
	@echo "Build libznn done."

znnd:
	go build -o $(BUILDDIR)/znnd $(SERVERMAIN)
	@echo "Build znnd done."
	@echo "Run \"$(BUILDDIR)/znnd\" to start znnd."

clean:
	rm -r $(BUILDDIR)/

all: znnd libznn

# Documentation targets — incremental v2 rollout on docs/coverage-v2.
# See docs/coverage-v2-conventions.md for godoc style.

docs: ## Serve godoc locally on http://localhost:6060
	@GOBIN_DIR="$$(go env GOBIN)"; \
	if [ -z "$$GOBIN_DIR" ]; then GOBIN_DIR="$$(go env GOPATH)/bin"; fi; \
	export PATH="$$GOBIN_DIR:$$PATH"; \
	if ! command -v godoc >/dev/null 2>&1; then \
		echo "godoc not found; installing golang.org/x/tools/cmd/godoc..."; \
		go install golang.org/x/tools/cmd/godoc@latest; \
	fi; \
	echo "godoc serving on http://localhost:6060/pkg/github.com/zenon-network/go-zenon/"; \
	godoc -http=:6060

doc-lint: ## Run godoc lint (revive exported + package-comments at warning severity)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found; install from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	}
	GOWORK=off golangci-lint run --config=.golangci.yml --issues-exit-code=0 ./...

doc-api: ## Regenerate static markdown API docs under docs/api/ (requires AGENTS.md, docs/STYLE.md — landed in final v2 PR)
	./scripts/gen-api-docs.sh
