# Stage 1 — Build the frontend
FROM node:20-alpine AS frontend

WORKDIR /build
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2 — Build the Go binary
FROM golang:1.26-alpine AS backend

WORKDIR /build

# Download dependencies (cached if go.mod / go.sum are unchanged)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o shelfarr ./cmd/server/

# Stage 3 — Minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=backend /build/shelfarr .
COPY --from=frontend /build/dist ./static

EXPOSE 8008

# SQLite database is stored here; mount a named volume to persist it.
VOLUME ["/data"]

CMD ["./shelfarr"]
