.PHONY: dev generate build docker run-docker tidy

## Generate Templ components (run after editing .templ files)
generate:
	templ generate

## Run the dev server with live-ish reloading (requires 'air' or just plain go run)
dev: generate
	mkdir -p tmp/audiobooks
	go run ./cmd/server

## Build production binary
build: generate
	CGO_ENABLED=1 go build -ldflags="-s -w" -o bin/shelfstone ./cmd/server

## Build Docker image
docker:
	docker build -t shelfstone:latest .

## Start via docker-compose
run-docker:
	docker compose up --build

## Tidy modules
tidy:
	go mod tidy

## Download Alpine.js (saves it locally so the container has it without a CDN)
fetch-alpine:
	mkdir -p web/static/js
	curl -sL https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js -o web/static/js/alpine.min.js

## Install templ CLI (requires go install already having run)
install-templ:
	go install github.com/a-h/templ/cmd/templ@v0.2.793

precommit:
	go fmt ./...
	go vet ./...