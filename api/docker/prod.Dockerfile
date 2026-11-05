# syntax=docker/dockerfile:1.7

FROM golang:1.26.10-alpine AS dependencies
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

FROM dependencies AS tools
RUN --mount=type=cache,target=/go/pkg/mod \
    GOBIN=/out go install \
    -tags="no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb" \
    github.com/pressly/goose/v3/cmd/goose@v3.27.1

FROM dependencies AS builder
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -buildid=" \
    -o /out/ai-translate-api \
    ./cmd/ai_translate

FROM alpine:3.21 AS final
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
WORKDIR /app
COPY --from=builder /out/ai-translate-api /app/ai-translate-api
COPY --from=tools /out/goose /usr/local/bin/goose
COPY docs/migrations /app/migrations
USER app
EXPOSE 4161
ENTRYPOINT ["/app/ai-translate-api"]
