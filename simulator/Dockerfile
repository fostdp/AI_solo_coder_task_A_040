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

RUN go build -ldflags="-s -w" -o /app/simulator .

FROM alpine:3.18

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl && \
    addgroup -S appuser && \
    adduser -S appuser -G appuser

ENV TZ=Asia/Shanghai
ENV APP_HOME=/app

WORKDIR ${APP_HOME}

COPY --from=builder /app/simulator ${APP_HOME}/simulator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

RUN chown -R appuser:appuser ${APP_HOME}

USER appuser

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD sh -c "pgrep -f simulator || exit 1"

ENTRYPOINT ["/app/simulator"]
