.PHONY: run test lint migrate-up migrate-down web-install web-dev web-build docs-serve docs-build run-dev run-dev-clean

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

web-install:
	npm --prefix web install

web-dev:
	npm --prefix web run dev

web-build:
	npm --prefix web run build

migrate-up:
	go run ./cmd/migrate -direction up

migrate-down:
	go run ./cmd/migrate -direction down

docs-serve:
	hugo server --bind 127.0.0.1 --baseURL http://127.0.0.1:1313/

docs-build:
	hugo --minify

run-dev:
	docker compose -f resources/docker-compose-dev-ephemeral.yaml up --build

run-dev-clean:
	docker compose -f resources/docker-compose-dev-ephemeral.yaml down -v --remove-orphans
	docker compose -f resources/docker-compose-dev-ephemeral.yaml build --no-cache --pull
	docker compose -f resources/docker-compose-dev-ephemeral.yaml up --force-recreate
