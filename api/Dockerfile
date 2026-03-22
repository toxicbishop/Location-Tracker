# ---- Build stage ----
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod first so Docker caches the dependency layer
# Only re-downloads if go.mod/go.sum changes
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o producer ./api

# ---- Runtime stage ----
# Use distroless — no shell, no package manager, minimal attack surface
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=builder /app/producer .

EXPOSE 8080

ENTRYPOINT ["/app/producer"]
