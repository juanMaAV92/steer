.PHONY: build test tidy lint

build:
	go build -o steer ./cmd/steer

test:
	go test ./...

tidy:
	go mod tidy

lint:
	gofmt -l . && test -z "$$(gofmt -l .)"
	go vet ./...
	golangci-lint run
