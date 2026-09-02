# Sprint 1

- HTTP API:
    - POST /api/tests — создать тест (scenario YAML в body)
    - POST /api/tests/{id}/run — создать и стартовать run (возвращает run_id)
    - POST /api/runs/{id}/metrics — агент шлёт метрики
    - GET /api/runs/{id}/stream — SSE со свежими метриками

- metadata as JSON files (data/tests/.yaml, data/runs/.json), 
- metrics as JSON lines files + SSE to UI.

- In-memory store с потоковой подпиской (SSE)
- Простая симуляция Runner, который шлёт метрики раз в секунду
- Unit-тесты для API + SSE
- Команды для запуска и тестирования