-include .env

export

export HTTP_READ_TIMEOUT=5s
export HTTP_READ_HEADER_TIMEOUT=2s
export HTTP_WRITE_TIMEOUT=10s
export HTTP_IDLE_TIMEOUT=60s
export IDEMPOTENCY_KEY_RETENTION=24h

.PHONY: migrate migrate-down migrate-status run test lint generate

migrate:
	goose -dir ./migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir ./migrations postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir ./migrations postgres "$(DATABASE_URL)" status

run:
	go run ./cmd/trip-service $(ARGS)

test:
	go test -race -count=2 ./...

lint:
	golangci-lint run

generate:
	go tool oapi-codegen \
	  -generate types,chi-server \
	  -package api \
	  -o internal/generated/api.gen.go \
	  contracts/openapi/trip-service.openapi.yaml
