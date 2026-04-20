BINARY    := ollama_exporter
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -X github.com/prometheus/common/version.Version=$(VERSION) \
             -X github.com/prometheus/common/version.Revision=$(shell git rev-parse --short HEAD 2>/dev/null) \
             -X github.com/prometheus/common/version.Branch=$(shell git branch --show-current 2>/dev/null) \
             -X github.com/prometheus/common/version.BuildDate=$(shell date -u +%Y%m%d-%H:%M:%S)

.PHONY: all build test lint docker clean

all: lint test build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/ollama_exporter/

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

docker:
	docker build -f deploy/docker/Dockerfile -t ollama_exporter:$(VERSION) .

clean:
	rm -f $(BINARY)
