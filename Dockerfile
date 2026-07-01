# ---------- Stage 1 : Build ----------
FROM golang:1.24 AS builder

WORKDIR /app

# Copy Go dependency files
COPY go.mod ./
COPY go.sum ./

# Download dependencies
RUN go mod download

# Copy application source code
COPY . .

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o app .

# ---------- Stage 2 : Runtime ----------
FROM ubuntu:24.04

WORKDIR /app

# Install trusted CA certificates for HTTPS communication
RUN apt-get update && \
apt-get install -y ca-certificates && \
update-ca-certificates && \
rm -rf /var/lib/apt/lists/*

# Copy binary from builder stage
COPY --from=builder /app/app .

# Copy HTML template
COPY --from=builder /app/index.html .

# Expose application port
EXPOSE 8080

# Start the application
CMD ["./app"]
