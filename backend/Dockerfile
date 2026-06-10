FROM golang:1.21-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata && \
    update-ca-certificates

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN go build -ldflags="-s -w -X main.version=1.0.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /app/gas-monitor .

FROM alpine:3.18

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    wget && \
    addgroup -S appuser && \
    adduser -S appuser -G appuser

ENV TZ=Asia/Shanghai
ENV APP_HOME=/app

WORKDIR ${APP_HOME}

COPY --from=builder /app/gas-monitor ${APP_HOME}/gas-monitor
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

RUN chown -R appuser:appuser ${APP_HOME}

USER appuser

EXPOSE 8080 6060

HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=10 \
    CMD wget --spider -q http://localhost:8080/api/health || exit 1

ENTRYPOINT ["/app/gas-monitor"]
