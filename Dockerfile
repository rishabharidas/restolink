# Stage 1: Build the Go binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git and build dependencies
RUN apk add --no-cache git ca-certificates

# Cache dependencies
COPY go.mod ./
RUN go mod download || true

# Copy source
COPY . .

# Download dependencies and tidy
RUN go mod tidy

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/restolink ./cmd/api

# Stage 2: Minimal runtime image
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/restolink /app/restolink

EXPOSE 8080

CMD ["/app/restolink"]
