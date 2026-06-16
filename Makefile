.PHONY: build test lint tidy

build:
	go build -o steer ./cmd/steer

test:
	go test ./...

tidy:
	go mod tidy
