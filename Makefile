.PHONY: all build run test test-go test-web cover lint fmt

all: lint test build

build:
	go build -o bin/wheel-tuner ./cmd/wheel-tuner

run:
	go run ./cmd/wheel-tuner -addr :8080 -db wheel-tuner.db

test: test-go test-web

test-go:
	go test -race ./...

test-web:
	cd web && node --test --experimental-test-coverage test/

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	golangci-lint fmt ./...
