# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o s3-monitor ./cmd/server

# Runtime stage
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates
RUN mkdir -p /app/data

COPY --from=builder /app/s3-monitor /app/s3-monitor
COPY --from=builder /app/web /app/web

ENV DB_PATH=/app/data/s3monitor.db
ENV TEMPLATE_DIR=/app/web/templates
ENV STATIC_DIR=/app/web/static
ENV PORT=8080

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/s3-monitor"]
