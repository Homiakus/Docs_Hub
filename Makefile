APP=docshub

.PHONY: run test test-cov build fmt lint clean

run:
	ADMIN_PASSWORD=dev-password-123 SESSION_SECRET=dev-secret-32-bytes-long-key go run ./cmd/docshub

test:
	go test ./... -count=1

test-cov:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out

build:
	go build -ldflags="-s -w" -o bin/$(APP) ./cmd/docshub

fmt:
	gofmt -w ./cmd ./internal

lint:
	go vet ./...

clean:
	rm -rf bin data coverage.out

all: lint test build
