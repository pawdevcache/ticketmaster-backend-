# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.22-alpine AS build
WORKDIR /src

# Dependencies first: this layer stays cached until go.mod/go.sum change,
# so ordinary source edits don't re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO off produces a fully static binary, so the runtime stage needs no libc.
# -trimpath keeps local build paths out of the binary; -s -w drop debug symbols.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/server

# --- runtime stage ---
FROM alpine:3.20

# TLS roots are required: MongoDB Atlas (mongodb+srv://) connections fail
# without them. Everything else the binary needs is compiled in.
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 app

WORKDIR /app
COPY --from=build /out/app /app/app

# Run unprivileged — nothing here needs root.
USER app

# tm.Port() reads PORT and falls back to 8080.
ENV PORT=8080
EXPOSE 8080

# Note this only proves the process is serving HTTP: /health answers 200 with
# "degraded" when the database is unreachable, by design, so the container
# stays up through a database blip instead of restart-looping.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT}/health" || exit 1

ENTRYPOINT ["/app/app"]
