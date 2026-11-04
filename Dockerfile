# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Install CA certificates and tzdata for secure HTTPS and timezone handling
RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build statically linked binary without debugging info for minimal image size
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static'" \
    -o /bin/shortlink \
    ./cmd/api

# Final runtime stage
FROM scratch

# Copy root CA certs for external outgoing HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Run as non-root user (nobody:nobody = 65534:65534)
USER 65534:65534

COPY --from=builder /bin/shortlink /bin/shortlink

EXPOSE 8080

ENTRYPOINT ["/bin/shortlink"]
