# ============================================================
# SmartLoadRunner — multi-stage Dockerfile
#
# Сборка:    docker build -t smart-load-runner .
# Запуск:    docker compose up -d  (рекомендуется)
#
# Для выбора бинарника используйте TARGET:
#   docker build --build-arg TARGET=coordinator -t slr-coordinator .
#   docker build --build-arg TARGET=runner -t slr-runner .
# ============================================================

# ------------------------------------------------------------
# Stage 1: build
# ------------------------------------------------------------
FROM golang:1.26-alpine AS build

ARG TARGET=coordinator
ARG VERSION=dev

# Инструменты для CGO-зависимостей (если понадобятся)
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Кэшируем слой модулей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY . .

# Сборка статического бинарника
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/app \
    ./cmd/${TARGET}

# ------------------------------------------------------------
# Stage 2: runtime — минимальный образ
# ------------------------------------------------------------
FROM alpine:3.22

# ca-certificates для HTTPS-запросов
RUN apk add --no-cache ca-certificates tzdata

# Не root пользователь
RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=build /out/app /app/app

# HEALTHCHECK задел (HTTP API координатора)
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

USER app

EXPOSE 8080 9090

ENTRYPOINT ["/app/app"]