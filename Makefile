VERSION ?= v0.1.0
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
  -X github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/app.version=$(VERSION) \
  -X github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/app.buildDate=$(BUILD_DATE) \
  -X github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/app.gitCommit=$(GIT_COMMIT)

# Contract: this repository is SOURCE for the community binary, not a user project.
# Bootstrap schemas are embedded at build time (internal/bootstrap/archive).
# `zqk system init` must only run against a separate project directory.

.PHONY: all build zqk zqk-community verify-bootstrap test clean

# Public product binary name is "zqk". Enterprise tools use distinguishing names.
all: zqk

# Prove embedded archive extracts into a temp project (never this repo).
verify-bootstrap:
	@test -f internal/bootstrap/archive/bootstrap.tar.gz || \
	  (echo "missing internal/bootstrap/archive/bootstrap.tar.gz — rebuild via Studio make bootstrap-archive and sync" >&2; exit 1)
	go test ./internal/bootstrap -count=1 -timeout 60s \
	  -run 'TestManifestPaths_EmbeddedArchive|TestExtractEmbeddedToTempProject'

zqk: verify-bootstrap
	@mkdir -p bin
	go build -ldflags '$(LDFLAGS)' -o bin/zqk ./cmd/zqk

# Legacy make target name (same product binary). Prefer `make zqk`.
zqk-community: zqk
	@cp -f bin/zqk bin/zqk-community

build: zqk

test: verify-bootstrap
	go test ./... -timeout 120s

clean:
	rm -rf bin/ dist-community/
