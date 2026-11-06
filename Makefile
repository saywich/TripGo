-include .env

export

export HTTP_READ_TIMEOUT=5s
export HTTP_READ_HEADER_TIMEOUT=2s
export HTTP_WRITE_TIMEOUT=10s
export HTTP_IDLE_TIMEOUT=60s
export IDEMPOTENCY_KEY_RETENTION=24h

.PHONY: migrate migrate-down migrate-status run test test-integration lint generate

migrate:
	goose -dir ./migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir ./migrations postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir ./migrations postgres "$(DATABASE_URL)" status

run:
	go run ./cmd/trip-service $(ARGS)

test:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	go test -race -tags=integration ./internal/repository/...

lint:
	golangci-lint run

generate:
	go generate ./internal/usecase/... ./internal/http/trip/...
	go tool oapi-codegen \
	  -generate types,chi-server \
	  -package api \
	  -o internal/generated/api.gen.go \
	  contracts/openapi/trip-service.openapi.yaml
