# SmartLoadRunner

Распределённая система нагрузочного тестирования. Состоит из двух сервисов:

| Сервис | Назначение | Порт |
|--------|-----------|------|
| **coordinator** | HTTP API: сценарии (YAML), тесты, раннеры, агрегация метрик, планировщик запусков | `8080` |
| **runner** | Агент нагрузки: исполняет VU-goroutines из сценария, аггрегирует RPS/перцентили, отправляет метрики координатору | `9090` |

---

## Быстрый старт

### 1. Требования

| Инструмент | Версия | Зачем |
|-----------|--------|-------|
| [Go](https://go.dev/dl/) | 1.25+ (проект на 1.26) | компиляция |
| [Task](https://taskfile.dev/installation/) | 3.x | оркестрация команд (`brew install go-task/tap/go-task`) |
| [Docker](https://www.docker.com/products/docker-desktop/) | любая актуальная | окружение БД и сервисов |

Опционально: [Air](https://github.com/air-verse/air) для hot-reload (`go install github.com/air-verse/air@latest`).

### 2. Клонирование и конфигурация

```bash
git clone git@github.com:wxwingim/SmartLoadRunner.git
cd SmartLoadRunner

task tidy          # скачать зависимости
cp .env.example .env   # создать локальное окружение
```

При необходимости отредактируйте `.env` (порты, пароли, уровни логов).

### 3. Запуск

#### Локально (без Docker, только код)

```bash
task build            # собрать coordinator и runner в bin/
task run:coordinator  # отдельный терминал: HTTP API на :8080
task run:runner       # отдельный терминал: агент на :9090
```

#### Локально + инфраструктура (рекомендуется для разработки)

```bash
task db:up            # поднять Postgres и Redis через Docker Compose
task dev              # оба процесса с hot-reload (нужен Air)
```

#### Полностью в Docker

```bash
task docker:up        # собрать и запустить окружение целиком
task docker:logs      # смотреть логи
task docker:down      # остановить всё
```

> ⚠️ `docker compose up -d --build` поднимет **только** БД (`postgres`, `redis`).
> Для запуска `coordinator` и `runner` в контейнерах используйте профиль `full`:
>
> ```bash
> docker compose --profile full up -d --build
> ```

---

## Команды Task

| Команда | Описание |
|---------|----------|
| `task build` | Собрать оба бинарника в `bin/` |
| `task run:coordinator` | Запустить coordinator локально |
| `task run:runner` | Запустить runner локально |
| `task dev` | Hot-reload обоих сервисов (нужен Air) |
| `task test` | Все тесты с `-race` и покрытием |
| `task test:unit` | Быстрые unit-тесты |
| `task test:integration` | Интеграционные тесты (нужны PG/Redis) |
| `task lint` | golangci-lint |
| `task format` | gofumpt + gci |
| `task tidy` | `go mod tidy` |
| `task db:up` / `db:down` | Поднять/остановить Postgres и Redis |
| `task db:psql` | Подключиться к Postgres |
| `task redis:cli` | Подключиться к Redis |
| `task docker:build` / `up` / `down` / `logs` | Управление Docker Compose |
| `task env:show` | Показать эффективные переменные окружения `SLR_*` |
| `task help` | Список всех команд |

Полный список: `task --list`

---

## Конфигурация

Вся конфигурация — через переменные окружения с префиксом `SLR_`.
Шаблон: [`.env.example`](.env.example). Код: [`internal/config`](internal/config/config.go).

Основные секции:

| Переменная | По умолчанию | Описание |
|-----------|-------------|----------|
| `SLR_ENVIRONMENT` | `development` | `development`, `staging`, `production` |
| `SLR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SLR_LOG_FORMAT` | `console` | `console`, `json` |
| `SLR_HTTP_PORT` | `8080` | порт API coordinator |
| `SLR_DB_HOST` / `SLR_DB_PORT` | `localhost` / `5432` | PostgreSQL |
| `SLR_REDIS_HOST` / `SLR_REDIS_PORT` | `localhost` / `6379` | Redis (задел) |
| `SLR_RUNNER_LISTEN_ADDR` | `:9090` | адрес агента |
| `SLR_RUNNER_CAPACITY` | `1000` | макс. VU на агенте |
| `SLR_RUNNER_REPORT_INTERVAL_SEC` | `1` | интервал отправки метрик |

---

## Docker

### Dockerfile

Multi-stage: `golang:1.26-alpine` (build) → `alpine:3.22` (runtime, non-root).

```bash
# Отдельный образ для сервиса
docker build --build-arg TARGET=coordinator -t slr-coordinator .
docker build --build-arg TARGET=runner -t slr-runner .
```

### Docker Compose

Сервисы:
- **postgres** — рекомендованный PostgreSQL 17
- **redis** — Redis 7 (задел под очереди)
- **coordinator** — API (профиль `full`)
- **runner** — агент нагрузки (профиль `full`)

Обычный workflow:

```bash
# 1. Только инфраструктура для локальной разработки
docker compose up -d postgres redis

# 2. Полное окружение
docker compose --profile full up -d --build
```

Масштабирование агентов:

```bash
docker compose --profile full up -d --scale runner=3
```

---

## Структура проекта

```
SmartLoadRunner/
├── cmd/
│   ├── coordinator/main.go    # HTTP API координатора
│   └── runner/main.go         # агент нагрузки
├── internal/
│   ├── app/                   # DI-контейнер и инициализация
│   ├── config/                # конфигурация из env (SLR_*)
│   ├── middleware/            # HTTP middleware
│   ├── models/                # доменные модели (Test, Run, Agent, MetricBucket)
│   ├── repository/            # доступ к данным (PG)
│   ├── scenario/              # парсер YAML-сценариев
│   ├── service/               # бизнес-логика
│   ├── store/                 # хранилище (in-memory / БД)
│   └── transport/             # протоколы (HTTP/gRPC/JSON)
├── api/                       # спецификации API (OpenAPI, protobuf)
├── migrations/                # SQL-миграции
├── pkg/errors/                # общие ошибки
├── scripts/                   # вспомогательные скрипты
├── docs/                      # документация и спеки
├── .env.example               # шаблон конфигурации
├── .golangci.yml              # линтер
├── Dockerfile                 # multi-stage сборка
├── docker-compose.yml         # окружение
└── Taskfile.yaml              # оркестрация команд
```

---

## Команды для каждого коммита

Перед коммитом выполните (можно одной строкой):

```bash
task format && task lint && task test
```

---

## Дорожная карта инструментов

| Этап | Инструмент | Статус |
|------|-----------|--------|
| 1. Сборка, линт, тесты | Task, golangci-lint, gofumpt, gci | ✅ настроено |
| 2. Конфигурация | `caarlos0/env`, `.env.example` | ✅ готово |
| 3. БД и кеш | PostgreSQL, Redis (docker-compose) | ✅ готово |
| 4. Контейнеризация | Dockerfile, docker-compose | ✅ готово |
| 5. HTTP API | echo/chi/gin (задел в `transport/`) | 📋 план |
| 6. Логирование | slog (стандартная библиотека) | 📋 план |
| 7. Миграции | golang-migrate | 📋 план |
| 8. CI | GitHub Actions (lint + test + build image) | 📋 план |
| 9. Hot-reload | Air | 📋 план |
| 10. gRPC между runner ↔ coordinator | protobuf | 📋 план |
| 11. Очереди событий | Redis Streams / NATS | 📋 план |
| 12. Межсервисные конфиги | Vault / AWS Secrets Manager | задел |
| 13. Обнаружение сервисов | Consul / etcd / Kubernetes | задел |

> Nginx не нужен на этапе разработки. Он появится позже как reverse-proxy перед API
> в production-развёртывании (или полностью заменится ingress в Kubernetes).

---

## FAQ

**Почему Task, а не Makefile?**
Taskfile проще: YAML, зависимости задач, проверки `status`, кроссплатформенность. Make остаётся для систем без Go.

**Почему нет .sh-скриптов?**
Начальная стадия не требует shell-обвязки. Все повторяемые операции уже есть в Taskfile, который кроссплатформенный. Скрипты появятся только для спец. случаев (например, установка инструментов CI-агентом).

**Как поднять только БД?**
`task db:up` — поднимет Postgres и Redis, код будет работать локально.

**Почему coordinator и runner в отдельных `main`?**
Это два независимых процесса: coordinator — центральный сервис, runner — агенты, которые масштабируются горизонтально.