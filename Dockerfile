# Stage 1: Build the static binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the statically linked binary
# Disable CGO to ensure it doesn't depend on glibc, and build for Linux
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /aixgo-sync \
    ./cmd/aixgo-sync

# Stage 2: Get CA certificates
FROM alpine:latest AS certs
RUN apk --no-cache add ca-certificates

# Stage 3: Final runtime stage using scratch
FROM scratch

# Copy CA certificates from the certs stage to enable outbound TLS
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Set environment variable so the Go binary knows where to find the CA bundle
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

# Copy the compiled statically linked binary from builder stage
COPY --from=builder /aixgo-sync /aixgo-sync

# Expose default port
EXPOSE 8080

# Run as non-root user (scratch doesn't have /etc/passwd by default, but we can use a numeric UID, e.g., 10001)
USER 10001:10001

# Set the entrypoint
ENTRYPOINT ["/aixgo-sync"]
CMD ["serve"]
