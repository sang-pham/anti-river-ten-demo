SHELL := /bin/bash

.PHONY: build run test tidy fmt vet swagger

build:
	go build ./...

run:
	go run ./cmd/api

test:
	go test ./... -race

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...


# Generate Swagger/OpenAPI docs into ./docs (requires swag CLI)
swagger:
	swag init -g cmd/api/main.go -o docs
