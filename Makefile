.PHONY: build test run fmt check install

build:
	mkdir -p bin
	go build -o bin/minstock ./cmd/minstock

test:
	go test ./...

run:
	go run ./cmd/minstock

fmt:
	gofmt -w cmd internal

check: fmt
	go test ./...
	go vet ./...

install:
	go install ./cmd/minstock
