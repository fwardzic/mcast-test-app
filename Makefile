IMAGE_NAME ?= mcast-test-app
IMAGE_TAG  ?= latest

.PHONY: build test lint clean build-static build-linux-amd64 build-linux-arm64 docker-build docker-buildx

build:
	go build -o bin/sender ./cmd/sender
	go build -o bin/receiver ./cmd/receiver

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags netgo -ldflags="-extldflags '-static'" -o bin/linux-amd64/sender ./cmd/sender
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags netgo -ldflags="-extldflags '-static'" -o bin/linux-amd64/receiver ./cmd/receiver

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags netgo -ldflags="-extldflags '-static'" -o bin/linux-arm64/sender ./cmd/sender
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags netgo -ldflags="-extldflags '-static'" -o bin/linux-arm64/receiver ./cmd/receiver

build-static: build-linux-amd64 build-linux-arm64

docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(IMAGE_NAME):$(IMAGE_TAG) .
