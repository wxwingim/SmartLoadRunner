# Sprint 0

MVP Smart Load Runner
Основная ценность: быстро и надёжно создавать воспроизводимые нагрузочные сценарии, запускать их локально/в CI/распределённо и получать понятные отчёты/алерты.
UX‑цель: минимальное трение от «настроить → запустить → понять результат» (time‑to‑first‑result ≈ 5–10 минут).

Dev (Frontend/Backend): хочет быстрый feedback, CLI/CI интеграцию, reproducibility.
SRE / Perf engineer: глубокая аналитика, distributed agents, интеграции (Prometheus/Grafana).
QA / Automation: CI assertions, visual editor/recorder.
Team Lead / PO: тренды, сравнения run’ов, cost control.

1. ## Ключевые пользовательские потоки (high‑level)
    Onboarding - One‑click Quickstart (prefilled demo).
    Create / Edit Test (YAML + visual recorder).
    Start Run (local / distributed / CI).
    Monitor Live Run (dashboard: RPS/latency/errors).
    Stop / Graceful shutdown.
    Post‑run analysis + Compare runs.
    CI integration, Admin flows (agents, secrets, quotas).


2. ## UX
- Onboarding/Quickstart
- Tests
    Список из карточек тестов (имя, last version, кнопки: run, edit, run history (last N) ).
    Фильтры (owner, project, status).

- Test Editor
    YAML редактор + визуальная форма (прим. подсветка)
    кнопка validate
    версионирование (сохраняет snapshot сценария)

- Run Modal
    Параметры запуска: VUs, Duration, Global rate (QPS), Seed (для reproducibility), Mode (local).
    Кнопка Start с предупреждением, если VUs/duration большие.
    Запуск возвращает run_id(task_id) и открывает Live Dashboard

- Live Dashboard
    - RPS (requests/sec);
    - Latency p50/p90/p99 (линии);
    - Error rate / count (бар);
    - Active VUs (gauge);
    - Recent errors list (сэмплы ответов, если allowed);

    - Кнопки Stop (graceful), Abort (force).
    
- Run Summary/Report
    Итоги: avg latency, p50/p90/p99, total requests, total errors, SLA pass/fail (по заданным threshold).
    Выгрузка csv/json
    Ссылка на raw samples (если сохранены), и Replay (re‑run with same seed).

- Simple CI integration 
    Example GitHub Action для headless run + failing build при нарушении SLA.

3. ## Архитектура (MVP → эволюция)
    Компоненты:
        - Frontend: Next.js (UI, editor, dashboard).
        - Coordinator: Go, REST API + SSE/WebSocket, run manager, file store (MVP).
        - Runner/Agent: Go binary, VU goroutines, per‑sec aggregation, отправка агрегатов Coordinator.
        - Storage: filesystem (data/tests/.yaml, data/runs/.json) → Postgres (metadata) + ClickHouse/Influx (timeseries).
        - Observability: Prometheus metrics, structured logs (zerolog/logrus), OpenTelemetry optional.

    Коммуникации:
        - MVP: Runner → POST /api/runs/:id/metrics; Coordinator → UI via SSE.
        - Scale: gRPC bidi streams (agent control + metrics), histogram merge (tdigest/HDR).

### Пример основных API
- POST /api/tests — сохранить тест (scenario YAML)
- POST /api/tests/:id/run — стартовать run → возвращает run_id
- GET /api/runs/:id/stream — SSE/WebSocket для live‑метрик
- POST /api/runs/:id/metrics — от агента: {ts, rps, histogram/percentiles, errors}

4. ## Run lifecycle & бизнес‑правила
- Состояния: DRAFT → SCHEDULED → STARTING → RUNNING → STOPPING → COMPLETED / FAILED / ABORTED.
- Rules:
    - Для публичных/production‑targets — require confirmation + allowlist.
    - Distributed: агент должен быть healthy и иметь capacity; Coordinator считает allocation.
    - Quotas: VU‑hours уменьшаются на старте, reconciliation на stop.
    - Reproducibility: каждый run хранит scenario, seed, env vars, agent/runner version, runner checksum.

- Data model (минимум)
    - Test: id, name, scenario_yaml, owner_id, created_at, latest_version.
    - Run: id, test_id, status, vus, duration, rate, start_at, stop_at, seed, agents_snapshot, metrics_summary_json, artifacts_links.
    - Agent: id, version, capacity, last_seen, labels.
    - MetricBucket: run_id, ts, rps, p50, p90, p99, errors, active_vus.
    - TraceSample (опционально, с retention/редакцией): run_id, ts, endpoint, latency, status_code, response_snippet.

- Безопасность и операции (must have)
    - Аутентификация/авторизация (OAuth2/JWT + RBAC).
    - Agent registration via token; mTLS для production agent↔coordinator.
    - Secrets encrypted at rest; redact response bodies по умолчанию.
    - Network isolation/egress policies для Runner.
    - Allowlist/denylist, double confirmation для публичных целей, rate limits per account.


- Product KPIs:
    - Activation: % users, who ran тест в 7 дней.
    - Time‑to‑first‑result (median).
    - Retention 30/90d.
    - Runs per team/month, avg VU‑hours/run.

- Platform KPIs:
    - Run start latency < 2s (local).
    - Agent heartbeat success > 99.9%.
    - Coordinator API p99 < 200ms.
    - Run completion reliability > 99% (runs <1h).

- Edge cases, ошибки и recovery
    - Agent offline при allocation: retry в timeout, потом FAILED + diagnostics.
    - Target unreachable: считать ошибки; abort only if > X% errors.
    - Clock skew: agents шлют monotonic counters + timestamps; reconciliation по-offset.
    - High resource usage: agents enforce per‑process limits; Coordinator мониторит health.

- Риски и mitigations
    - Abuse / DDoS risk: target verification (DNS token), allowlist, quotas, manual review for large runs.
    - Inaccurate percentiles: use tdigest/HDR merges, документировать точность.
    - Data privacy: redact по‑умолчанию, opt‑in raw capture.

5. ## Roadmap и приоритеты (MVP → next)

- MVP (must):
    - Repo skeleton + docker‑compose (coordinator + runner sample).
    - Local runner + POST metrics → Coordinator + SSE → UI live dashboard.
    - Scenario validation + dry‑run (1 VU × 1 iter), run summary, export, basic CI example.
    - Security basics: agent tokens, secrets encrypt, allowlist.

- Next (high)
    - Distributed agents + gRPC bidi + mTLS.
    - CI badges + reproducible runs + run comparison UI.
    - Visual recorder/editor + assertions engine.
    - Long term storage: Postgres + ClickHouse/Influx + advanced analytics.

Further: autoscaling agents (k8s operator), multi‑region runs, paid features (retention, private agents).

