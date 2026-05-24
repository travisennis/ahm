build:
	mkdir -p bin
	go build -o bin/ahm ./cmd/ahm

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

ci:
	go test ./...
	go vet ./...
