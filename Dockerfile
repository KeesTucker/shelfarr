# Stage 1 — Build the frontend
FROM node:25-alpine AS frontend

WORKDIR /build
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2 — Build the Go binary
FROM golang:1.25.7-alpine AS backend

WORKDIR /build

# Download dependencies (cached if go.mod / go.sum are unchanged)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o shelfarr ./cmd/server/

# Stage 3 — Minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

# Create a non-root user/group. The UID/GID can be overridden at runtime via
# the compose `user:` field (e.g. PUID=2000 PGID=2000).
RUN addgroup -S -g 1000 shelfarr \
 && adduser  -S -u 1000 -G shelfarr shelfarr

WORKDIR /app
COPY --from=backend /build/shelfarr .
COPY --from=frontend /build/dist ./static
RUN chown -R shelfarr:shelfarr /app

EXPOSE 8008

# SQLite database is stored here; mount a named volume to persist it.
VOLUME ["/data"]

USER shelfarr
CMD ["./shelfarr"]
