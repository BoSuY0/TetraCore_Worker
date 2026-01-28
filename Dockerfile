# ---- Stage 1: Build ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Кешуємо залежності окремим шаром
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /worker ./cmd/worker

# ---- Stage 2: Runtime ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Не-привілейований користувач
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /worker /worker
COPY config/worker.yaml /app/config/worker.yaml

USER appuser

# Worker — WebSocket клієнт, а не сервер; порти не експортуємо.
# Метрики (опціонально) можна відкрити через docker-compose / k8s.

CMD ["/worker"]
