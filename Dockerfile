# Builds the production image: a single static Go binary with the SQLite
# driver compiled in (modernc.org/sqlite is pure Go, no CGO needed) and
# templates/static assets embedded via go:embed — no files to COPY at runtime.
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN mkdir -p /appdata

# distroless static — no shell, no libc, ca-certs included for SMTP TLS.
# :nonroot runs as UID 65532. SQLite's DB file lives on a mounted volume at
# /data (see docker-compose.yml / the dadock service definition).
FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app
COPY --from=builder /out/server .
COPY --from=builder --chown=65532:65532 /appdata /data

ENV PORT=8080
ENV DB_PATH=/data/rafactools.db
EXPOSE 8080

ENTRYPOINT ["./server"]
