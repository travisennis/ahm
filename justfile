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

lint:
	"$(go env GOPATH)/bin/golangci-lint" run

vuln:
	"$(go env GOPATH)/bin/govulncheck" ./...

release-check:
	"$(go env GOPATH)/bin/goreleaser" check
	"$(go env GOPATH)/bin/goreleaser" release --snapshot --clean --skip publish

fix: tidy fmt

ci: fmt-check tidy-check vet test-race lint vuln build release-check

verify: ci

install-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{ golangci_lint_version }}
	go install golang.org/x/vuln/cmd/govulncheck@{{ govulncheck_version }}
	go install github.com/goreleaser/goreleaser/v2@{{ goreleaser_version }}

quick:
	go test ./...
	go vet ./...
