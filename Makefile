.PHONY: run test lint migrate-up migrate-down

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

migrate-up:
	@echo "migrate-up: not yet implemented"

migrate-down:
	@echo "migrate-down: not yet implemented"
