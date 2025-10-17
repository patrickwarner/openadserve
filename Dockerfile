# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations for smaller binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o openadserve ./cmd/server/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/openadserve .

# Copy necessary data files
COPY --from=builder /app/data ./data
COPY --from=builder /app/static ./static

EXPOSE 8787

CMD ["./openadserve"]
