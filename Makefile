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

# Documentation targets. See docs/STYLE.md for conventions.

docs: ## Serve godoc locally on http://localhost:6060
	@command -v godoc >/dev/null 2>&1 || { \
		echo "godoc not found; installing golang.org/x/tools/cmd/godoc..."; \
		go install golang.org/x/tools/cmd/godoc@latest; \
	}
	@echo "godoc serving on http://localhost:6060/pkg/github.com/zenon-network/go-zenon/"
	godoc -http=:6060

doc-lint: ## Run godoc lint (revive: exported, package-comments)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found; install from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	}
	golangci-lint run --config=.golangci.yml ./...

doc-api: ## Regenerate static markdown API docs under docs/api/
	./scripts/gen-api-docs.sh
