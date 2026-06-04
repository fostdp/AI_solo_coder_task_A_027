FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION:-1.0.0} -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /climate-backend \
    main.go

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 app && \
    adduser -u 1000 -G app -D app
WORKDIR /app
COPY --from=builder /climate-backend .
RUN chown -R app:app /app
USER app
EXPOSE 8080 6060
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1
CMD ["./climate-backend"]
