# Dockerfile Parser Test Fixture
# Tests symbol extraction for container definitions
# Line numbers are predictable for UPTS validation

# === Pattern: Build arguments ===
# Line 6-8
ARG GO_VERSION=1.22
ARG ALPINE_VERSION=3.19
ARG APP_NAME=mdemg-parser

# === Pattern: Base image (build stage) ===
# Line 11
FROM golang:${GO_VERSION}-alpine AS builder

# === Pattern: Labels ===
# Line 14-17
LABEL maintainer="reh3376"
LABEL version="1.0.0"
LABEL description="MDEMG Parser Service"
LABEL org.opencontainers.image.source="https://github.com/example/mdemg"

# === Pattern: Environment variables ===
# Line 20-24
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
ENV APP_ENV=production
ENV LOG_LEVEL=info

# === Pattern: Working directory ===
# Line 27
WORKDIR /app

# === Pattern: Copy and Run ===
# Line 30-35
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /bin/app ./cmd/server

# === Pattern: Runtime stage ===
# Line 38
FROM alpine:${ALPINE_VERSION} AS runtime

# === Pattern: Runtime environment ===
# Line 41-44
ENV APP_PORT=8080
ENV APP_HOST=0.0.0.0
ENV CONFIG_PATH=/etc/app/config.yaml
ENV DATA_DIR=/var/lib/app

# === Pattern: Expose ports ===
# Line 47-48
EXPOSE 8080
EXPOSE 9090

# === Pattern: Volume mounts ===
# Line 51-52
VOLUME /etc/app
VOLUME /var/lib/app

# === Pattern: Copy from builder ===
# Line 55
COPY --from=builder /bin/app /usr/local/bin/app

# === Pattern: Health check ===
# Line 58-62
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# === Pattern: User ===
# Line 65
USER 1000:1000

# === Pattern: Entrypoint and CMD ===
# Line 68-69
ENTRYPOINT ["/usr/local/bin/app"]
CMD ["--config", "/etc/app/config.yaml", "serve"]
