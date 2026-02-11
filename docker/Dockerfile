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

# Build all binaries with version injection
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X 'github.com/bobmccarthy/vire/internal/common.Version=${VERSION}' \
    -X 'github.com/bobmccarthy/vire/internal/common.Build=${BUILD}' \
    -X 'github.com/bobmccarthy/vire/internal/common.GitCommit=${GIT_COMMIT}'" \
    -o vire-server ./cmd/vire-server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X 'github.com/bobmccarthy/vire/internal/common.Version=${VERSION}' \
    -X 'github.com/bobmccarthy/vire/internal/common.Build=${BUILD}' \
    -X 'github.com/bobmccarthy/vire/internal/common.GitCommit=${GIT_COMMIT}'" \
    -o vire-mcp ./cmd/vire-mcp

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o openai-proxy ./cmd/openai-proxy

# Resolve config: prefer docker/vire.toml (local, with keys) over docker/vire.toml.docker (no keys)
RUN if [ -f docker/vire.toml ]; then cp docker/vire.toml vire.toml; else cp docker/vire.toml.docker vire.toml; fi

# Runtime stage
FROM alpine:3.19

LABEL org.opencontainers.image.source="https://github.com/bobmcallan/vire"

WORKDIR /app

# Install ca-certificates for HTTPS requests and wget for healthcheck
RUN apk --no-cache add ca-certificates wget

# Copy binaries from builder
COPY --from=builder /build/vire-server .
COPY --from=builder /build/vire-mcp .
COPY --from=builder /build/openai-proxy .

# Copy resolved config and version file
COPY --from=builder /build/vire.toml .
COPY .version .

# Create data and logs directories
RUN mkdir -p /app/data /app/logs

# Expose HTTP ports (REST API + MCP + OpenAI Proxy)
EXPOSE 4242 4243 4244

# No default health check — each service defines its own in docker-compose.yml
# (vire-server on :4242, vire-mcp on :4243)
HEALTHCHECK NONE

ENTRYPOINT ["./vire-server"]
