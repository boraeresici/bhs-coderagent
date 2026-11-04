VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build build-arm64 test vet fmt check

build:
	go build -ldflags "$(LDFLAGS)" -o bin/coderagent ./cmd/coderagent

# VPS arm64; sunucuda derleme yok (ANALIZ §8.2).
build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/coderagent-linux-arm64 ./cmd/coderagent

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "gofmt gerekli"; exit 1)
	go vet ./...
	go test ./...
