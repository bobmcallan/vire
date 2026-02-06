# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Version build arguments
ARG VERSION=dev
ARG BUILD=unknown
ARG GIT_COMMIT=unknown

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with version injection
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X 'github.com/bobmccarthy/vire/internal/common.Version=${VERSION}' \
    -X 'github.com/bobmccarthy/vire/internal/common.Build=${BUILD}' \
    -X 'github.com/bobmccarthy/vire/internal/common.GitCommit=${GIT_COMMIT}'" \
    -o vire-mcp ./cmd/vire-mcp

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /build/vire-mcp .

# Copy config and version file
COPY docker/vire.toml .
COPY .version .

# Create data directory
RUN mkdir -p /app/data

# Expose SSE port
EXPOSE 4242

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:4242/sse || exit 1

# Run the server
CMD ["./vire-mcp"]
