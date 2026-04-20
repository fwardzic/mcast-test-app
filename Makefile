.PHONY: build test lint clean

build:
	go build -o bin/sender ./cmd/sender
	go build -o bin/receiver ./cmd/receiver

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
