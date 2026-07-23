.PHONY: build test test-race lint run clean dist dist-linux dist-linux-arm64 dist-windows dist-windows-arm64 dist-macos dist-macos-arm64 dist-all

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)
DIST := build/dist

build:
	go build ./cmd/eebustracer

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run

run:
	go run ./cmd/eebustracer serve

clean:
	rm -f eebustracer
	rm -rf build/

# Cross-compiled release binaries. Runtime is pure Go (no CGO), so these
# just work from any host OS — no cross-toolchain needed.
$(DIST):
	@mkdir -p $(DIST)

dist-linux: | $(DIST)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/eebustracer-linux-amd64 ./cmd/eebustracer

dist-linux-arm64: | $(DIST)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/eebustracer-linux-arm64 ./cmd/eebustracer

dist-windows: | $(DIST)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/eebustracer-windows-amd64.exe ./cmd/eebustracer

dist-windows-arm64: | $(DIST)
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/eebustracer-windows-arm64.exe ./cmd/eebustracer

dist-macos: | $(DIST)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/eebustracer-macos-amd64 ./cmd/eebustracer

dist-macos-arm64: | $(DIST)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/eebustracer-macos-arm64 ./cmd/eebustracer

# Convenience: Linux + Windows amd64 only (the common desktop targets).
dist: dist-linux dist-windows

# Every supported target.
dist-all: dist-linux dist-linux-arm64 dist-windows dist-windows-arm64 dist-macos dist-macos-arm64
	@echo "Built:"
	@ls -lh $(DIST)/
