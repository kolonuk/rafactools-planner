# Builds the production image: a single static Go binary with the SQLite
# driver compiled in (modernc.org/sqlite is pure Go, no CGO needed) and
# templates/static assets embedded via go:embed — no files to COPY at runtime.
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# VERSION -> internal/version.Version (shown in the web and PDF footers).
# "1.<PR number>" set by deploy.yml for a real build; defaults to "dev" for
# a local `docker build .` with no --build-arg.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-X rafactools-planner/internal/version.Version=${VERSION}" \
      -o /out/server ./cmd/server
RUN mkdir -p /appdata

# distroless static — no shell, no libc, ca-certs included for SMTP TLS.
# :nonroot runs as UID 65532. SQLite's DB file lives on a mounted volume at
# /data (see docker-compose.yml).
FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app
COPY --from=builder /out/server .
COPY --from=builder --chown=65532:65532 /appdata /data

# Explicit, even though the :nonroot image already defaults to this user: scanners
# (Trivy DS-0002) read the Dockerfile, not the base image.
USER 65532:65532

ENV PORT=8080
ENV DB_PATH=/data/rafactools.db
EXPOSE 8080

ENTRYPOINT ["./server"]
