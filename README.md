# GoDelivery Demo

Рабочая распределённая система для интерактивного семинара по архитектуре и конструированию ПО. Студент и преподаватель используют **один код**: выполняют сценарии, исследуют API, БД, Kafka, логи, метрики и trace.

## Бизнес-сценарий

Клиент выбирает пиццу, создаёт и оплачивает заказ. Заказ проходит `Paid → Accepted → Cooking → Ready → Delivering → Completed`. `tok-decline` воспроизводит отказ оплаты; Fulfillment не запускается. Исходная User Story и GIVEN/WHEN/THEN: [docs/user-story.md](docs/user-story.md).

## Архитектура

Ровно три бизнес-сервиса на Go: **Order**, **Payment**, **Fulfillment**. Order вызывает Payment синхронно по REST. Kafka передаёт `order.paid.v1` в Fulfillment и `fulfillment.status-changed.v1` обратно в Order. Каждый сервис владеет собственной БД; один PostgreSQL-контейнер содержит `order_db`, `payment_db`, `fulfillment_db`, отдельные роли и права CONNECT.

HTTP, Kafka и SQL создают OpenTelemetry spans. Структурированные slog JSON-логи и метрики экспортируются вместе с traces через OTLP/HTTP в Collector. Prometheus, Loki и Jaeger хранят соответствующие сигналы; Grafana имеет готовые datasources и три dashboards.

Kitchen и Delivery объединены в Fulfillment **для обучения**. В реальной системе это могут быть разные bounded contexts. Layout, outbox, at-least-once и ограничения: [docs/design.md](docs/design.md). [PlantUML/C4](docs/architecture/README.md).

## Требования

- Docker Engine/Desktop с Compose v2 (рекомендуется 2.39+; нужны `--wait` и условия startup).
- `make`; для ручных HTTP-команд — `curl`. Windows: WSL2 либо терминал с make.
- Рекомендуется 4 CPU, 8 GB доступной Docker памяти, 8 GB свободного диска.
- Свободные локальные порты: 8081–8083, 3000, 16686, 5432, 19092, 4318, 8889, 13133, 9090, 3100.
- Интернет нужен при первой сборке для pinned images и Go modules. Swagger assets входят в архив; CDN не нужен. Go, jq, Python, protoc и Kafka CLI устанавливать на host не требуется.

## Quick Start

```sh
unzip go-delivery-demo.zip
cd go-delivery-demo
make up
make demo
make traffic
make observe
```

`make up` автоматически создаёт базы/роли/topics, применяет go-migrate UP migrations, provision'ит Grafana, ждёт healthchecks и дополнительно проверяет все компоненты. При первой сборке загрузка зависимостей может занять несколько минут. Если сборка завершилась, но runtime ожидание истекло: `make ps`, `make logs`, затем повторить `make up`.

Для уверенного показа RPS и percentiles: `make traffic` (примерно 2 минуты), затем выбрать последние 15 минут в Grafana. Пустой график до появления измерений не означает сбой.

## URLs

| Компонент | Адрес |
|---|---|
| Order Swagger | http://localhost:8081/swagger/ |
| Payment Swagger | http://localhost:8082/swagger/ |
| Fulfillment Swagger | http://localhost:8083/swagger/ |
| Technical Dashboard | http://localhost:3000/d/godelivery-technical |
| Business Dashboard | http://localhost:3000/d/godelivery-business |
| Runtime Traffic Map | http://localhost:3000/d/godelivery-traffic |
| Grafana Explore | http://localhost:3000/explore |
| Jaeger | http://localhost:16686 |
| Prometheus | http://localhost:9090 |
| Loki ready | http://localhost:3100/ready |
| Collector health | http://localhost:13133/ |

Grafana открывается анонимно с правами Viewer на локальном стенде. В конфигурации используются только демонстрационные credentials из `.env.example`/Compose. Опциональный `.env` позволяет изменить пароли **перед первым запуском**; существующий volume уже содержит прежние роли. Стенд предназначен для localhost: порты привязаны к loopback.

## Основные команды

| Команда | Назначение |
|---|---|
| `make help` | Полный список |
| `make up / down / reset` | Запуск / остановка / удаление всех demo-данных |
| `make build` | Сборка сервисов и инструментов |
| `make health / ps / logs` | Проверка состояния |
| `make demo` | Happy + declined path с REST/SQL/Kafka assertions |
| `make traffic` | Успешные и failed заказы для dashboards |
| `make observe` | Проверка экспортированных сигналов после demo |
| `make checkout / advance / failure` | Отдельные управляемые шаги |
| `make kafka-events / kafka-groups` | Payload, headers, partitions, offsets / group lag |
| `make db-orders / db-payments / db-fulfillment` | Таблицы каждого владельца |
| `make migrate-up / migrate-down` | Schema versioning; DOWN удаляет данные |

`make reset` удаляет volumes только этого Compose-проекта и demo state. Для проверки с чистого состояния: `sh scripts/clean-check.sh`.

## Тесты

```sh
make test                 # domain/usecase + HTTP httptest, race detector
make lint                 # форматирование и vet
make test-integration     # после make up: PostgreSQL, Kafka, E2E, telemetry
```

Repository tests создают отдельную schema в собственной БД и проверяют UP/DOWN, idempotency, rollback и lifecycle. Kafka test создаёт уникальный topic/group и проверяет propagation + committed offset. `make test-integration` затем запускает бизнес-E2E и проверку observability. Ошибки приводят к ненулевому exit code, а не к успешному skip.

Если Go установлен локально: `go test ./...`, `go vet ./...`, `go build ./...`; для интеграционных тестов задайте `ORDER_DB_URL`, `PAYMENT_DB_URL`, `FULFILLMENT_DB_URL`, `KAFKA_BROKER` и выполните `go test -tags=integration -p 1 ./...`.

Фактически выполненные и недоступные проверки поставки перечислены в [VALIDATION.md](VALIDATION.md).

## Проведение семинара

Откройте [SEMINAR_GUIDE.md](SEMINAR_GUIDE.md). Он содержит 25 последовательных шагов с конкретными curl, SQL/Kafka командами, запросами Loki/Prometheus, ссылками на функции и вопросами. Упражнения — исследование работающей системы; заполнять код для основного сценария не требуется. Полный маршрут рассчитан примерно на 150–180 минут либо два занятия по 90 минут.

## Структура

- `services/{order,payment,fulfillment}/cmd/<service>/main.go` — Composition Root.
- `services/*/internal/{domain,usecase,repository,delivery}` — понятные слои.
- `services/*/api/openapi.yaml`, `services/*/migrations` — transport и schema contracts.
- `contracts/events` — самостоятельные versioned Kafka DTO и их описание.
- `internal/platform` — транспорт/telemetry/runtime, без общих business repositories.
- `cmd/demo`, `cmd/probe` — demo/traffic/проверки и healthcheck binary.
- `deploy` — конфигурация observability, Grafana provisioning, локальные Swagger assets.
- `docs` — User Story, C4/sequence, observability, trade-offs.
- `scripts` — smoke, traffic, integration и clean-start.

Версии образов закреплены, `latest` не используется. [docs/versions.md](docs/versions.md) содержит версии и первичные источники.
