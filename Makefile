APP=docshub

.PHONY: run test build fmt clean
run:
	go run ./cmd/docshub

test:
	go test ./...

build:
	go build -o bin/$(APP) ./cmd/docshub

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf bin data
