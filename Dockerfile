# Multi-stage build for optimized production image
FROM golang:1.23.4 as build

WORKDIR /app

COPY . ./   
RUN go mod download


# Build the application with version information
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=${VERSION}" \
    -o scanoss-hfh-api \
    ./cmd/server

# Production image
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

WORKDIR /app

# Create necessary directories
RUN mkdir -p /app/config /app/snapshots /var/log/scanoss

# Copy the binary from build stage
COPY --from=build /app/scanoss-hfh-api /app/scanoss-hfh-api

# Create non-root user for security
RUN groupadd -r scanoss && useradd -r -g scanoss scanoss
RUN chown -R scanoss:scanoss /app /var/log/scanoss

# Switch to non-root user
USER scanoss

# Health check for the REST API
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:40061 || exit 1

# Expose ports
EXPOSE 40061 50061 60061

# Default entrypoint
ENTRYPOINT ["./scanoss-hfh-api"]

# Default command (can be overridden by docker-compose)
CMD ["--help"]
