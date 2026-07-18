APP=docshub

.PHONY: run test test-cov build fmt lint clean

run:
	@test -n "$$ADMIN_PASSWORD" || (printf '%s\n' 'ADMIN_PASSWORD is required' >&2; exit 1)
	@test -n "$$SESSION_SECRET" || (printf '%s\n' 'SESSION_SECRET is required' >&2; exit 1)
	go run ./cmd/docshub

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
