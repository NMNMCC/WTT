# --- Build Stage ---
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy Go modules and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o wtt-server ./cmd/wtt-server

# --- Final Stage ---
FROM alpine:latest

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/wtt-server .

# Expose the port the server runs on
EXPOSE 8080

# Command to run the server
CMD ["./wtt-server"]
