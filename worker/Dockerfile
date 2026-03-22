# ---- Build stage ----
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o consumer ./worker

# ---- Runtime stage ----
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=builder /app/consumer .

ENTRYPOINT ["/app/consumer"]
