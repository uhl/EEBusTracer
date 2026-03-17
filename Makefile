.PHONY: build test test-race lint run clean

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
