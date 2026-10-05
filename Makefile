VERSION ?= 0.4.0-readonly
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test run fmt check install

build:
	mkdir -p bin
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/minstock ./cmd/minstock

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
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/minstock
