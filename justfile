golangci_lint_version := "v2.12.2"
goreleaser_version := "v2.16.0"
govulncheck_version := "v1.3.0"

# Reject branch names outside the <type>/<slug> convention.
# just interpolates a recipe argument into shell source, so an otherwise legal
# Git ref such as `feat/x$(...)` would execute before Git ever saw it. This
# check keeps the accepted character set narrow enough that quoting it has
# nothing to defend.
_check-branch-name name:
    @name={{ quote(name) }}; \
    case "$name" in \
        -*|/*|*/|*..*|*.lock|*/.*|*.|*[!A-Za-z0-9._/-]*) \
            echo "ERROR: branch name may use only letters, digits, dot, underscore, hyphen, and /, and may not start with '-' or '/', end with '/', or contain '..'" >&2; \
            exit 1 ;; \
    esac; \
    case "${name%%/*}" in \
        feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert) ;; \
        *) echo "ERROR: branch name must start with a commit type, for example feat/task-timeouts" >&2; exit 1 ;; \
    esac; \
    case "$name" in \
        */*) ;; \
        *) echo "ERROR: branch name must be <type>/<slug>, for example feat/task-timeouts" >&2; exit 1 ;; \
    esac

# Local master is the base because it may carry unpushed commits that cutting
# from origin/master would drop.
# Start a task branch cut from an up-to-date master.
branch name: (_check-branch-name name)
    @git switch master
    @git pull --ff-only
    @git switch --create {{ quote(name) }} --no-track

# Rebase <name> onto master and fast-forward, keeping history linear.
integrate name: (_check-branch-name name)
    #!/bin/sh
    set -eu
    name={{ quote(name) }}
    test "$(git branch --show-current)" = master || { echo "ERROR: run integrate from master" >&2; exit 1; }
    test -z "$(git status --porcelain)" || { echo "ERROR: worktree must be clean before integrating" >&2; exit 1; }
    git rev-parse --verify --quiet "refs/heads/$name" >/dev/null || { echo "ERROR: no such branch: $name" >&2; exit 1; }
    if git merge-base --is-ancestor "$name" master; then echo "ERROR: $name has no commits to integrate (already contained in master)" >&2; exit 1; fi
    git pull --ff-only
    git switch "$name"
    git rebase master
    if test -z "$(git rev-list "master..$name")"; then
        echo "NOTE: $name had no commits left after rebase (already applied); master is unchanged" >&2
        git switch master
        exit 0
    fi
    git switch master
    git merge --ff-only "$name"

install:
    go install -trimpath ./cmd/ahm

build:
    mkdir -p bin
    go build -trimpath -o bin/ahm ./cmd/ahm

test:
    go test ./...

# Check the documented CLI command, flag, and alias inventories against the
# Cobra command tree. Part of `just test`; run it after changing CLI wiring.
cli-parity:
    go test ./internal/ahm -run 'TestCLIDocumentationParity|TestInventoryDrift'

test-race:
    go test -race -cover ./...

vet:
    go vet ./...

fmt:
    go fmt ./...

fmt-check:
    test -z "$(gofmt -l .)"

tidy:
    go mod tidy

tidy-check:
    go mod tidy -diff

update-deps:
    go get -u ./...
    go mod tidy

lint:
    "$(go env GOPATH)/bin/golangci-lint" run

vuln:
    "$(go env GOPATH)/bin/govulncheck" ./...

# Lint markdown files for structural issues. Requires Node.js (npx).
docs-md-lint:
    npx --yes markdownlint-cli2 "**/*.md"

release-check:
    "$(go env GOPATH)/bin/goreleaser" check
    "$(go env GOPATH)/bin/goreleaser" release --snapshot --clean --skip publish

prepare-release version="":
    ./scripts/prepare-release.sh {{ version }}

fix: tidy fmt

ci: fmt-check tidy-check vet test-race lint vuln docs-md-lint build release-check

verify: ci

install-tools:
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{ golangci_lint_version }}
    go install golang.org/x/vuln/cmd/govulncheck@{{ govulncheck_version }}
    go install github.com/goreleaser/goreleaser/v2@{{ goreleaser_version }}

quick:
    go test ./...
    go vet ./...
