.PHONY: run test lint migrate-up migrate-down

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

migrate-up:
	go run ./cmd/migrate -direction up

migrate-down:
	go run ./cmd/migrate -direction down
