.PHONY: run fmt lint test test-integration check sqlc migrate-up migrate-down db-up db-down tools

run:
	go run ./cmd/api

fmt:
	gofmt -w ./...
	goimports -w ./...

lint:
	golangci-lint run ./...

test:
	go test -race ./...

test-integration:
	go test -race -tags=integration ./...

check: lint test test-integration

sqlc:
	sqlc generate

migrate-up:
	goose -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir db/migrations postgres "$(DATABASE_URL)" down

db-up:
	docker compose up -d postgres

db-down:
	docker compose down
