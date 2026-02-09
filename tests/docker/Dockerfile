# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o vire-mcp ./cmd/vire-mcp

# Runtime stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates
RUN adduser -D -h /app testuser

COPY --from=builder /build/vire-mcp .
COPY tests/docker/vire.toml ./vire.toml
COPY tests/docker/vire-blank.toml ./vire-blank.toml

RUN mkdir -p /app/data /app/logs && chown -R testuser:testuser /app

USER testuser

CMD ["tail", "-f", "/dev/null"]
