SHELL := /bin/bash

.PHONY: build run test tidy fmt vet

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
