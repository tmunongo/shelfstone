# ---- Build stage ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Install templ code generator
RUN go install github.com/a-h/templ/cmd/templ@v0.2.793

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate Templ components
RUN templ generate

# Build the binary (CGO required for go-sqlite3)
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o shelfstone ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.19

RUN apk add --no-cache sqlite-libs ca-certificates tzdata ffmpeg

WORKDIR /app

COPY --from=builder /build/shelfstone .
COPY --from=builder /build/web/static ./web/static

# Create volume mount points
RUN mkdir -p /data/audiobooks /data

VOLUME ["/data"]

EXPOSE 8080

ENV AUDIOBOOK_DATA_DIR=/data/audiobooks \
    DATABASE_PATH=/data/shelfstone.db   \
    APP_PORT=8080                        \
    SCAN_INTERVAL_MINUTES=60

ENTRYPOINT ["./shelfstone"]
