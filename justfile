golangci_lint_version := "v2.12.2"
goreleaser_version := "v2.16.0"
govulncheck_version := "v1.3.0"

install:
    go install -trimpath ./cmd/ahm

build:
    mkdir -p bin
    go build -trimpath -o bin/ahm ./cmd/ahm

test:
    go test ./...

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

# Refresh golden agent transcripts from the real agent CLIs. Makes real LLM
# calls (costs money); run manually after agent upgrades, never in CI.
capture-agent-fixtures:
    ./scripts/capture-agent-fixtures.sh

# Live agent smoke test: runs each installed agent CLI end-to-end through
# `ahm task work` (a few real LLM calls per agent; costs money). Run after
# changing agent arg builders, parsers, or orchestration; not part of `ci`.
smoke-agents:
    AHM_AGENT_SMOKE=1 go test ./internal/ahm -run TestAgentSmoke -v -count=1 -timeout 30m

quick:
    go test ./...
    go vet ./...
