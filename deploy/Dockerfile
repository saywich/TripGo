FROM golang:1.25.4-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/trip-service \
    ./cmd/trip-service

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/trip-service /trip-service

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/trip-service"]
