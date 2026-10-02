# SEMINAR_GUIDE — один работающий GoDelivery

Клиент хочет выбрать еду, оплатить заказ и дождаться доставки. В этом семинаре каждое утверждение о пути заказа проверяем конкретным API response, SQL row, Kafka event или telemetry signal. Код уже работает; преподаватель и студент исследуют одну реализацию.

Маршрут: User Story → C4 → Swagger → checkout → REST Payment → SQL → Kafka → Fulfillment → Order Completed → logs → metrics → trace → SQL span → runtime map → declined payment → расследование.

Полный маршрут — 150–180 минут или два занятия по 90. До занятия преподаватель выполняет `make up && make demo && make observe`; отчёт доступных проверок поставки — [VALIDATION.md](VALIDATION.md). Студенты начинают ниже. Команды выполняются из корня репозитория в одном POSIX terminal; go/jq/Python на host не нужны. Цены в целых рублях; customer/restaurant фиксированы, идентификаторы заказов уникальны. Не используйте `curl --fail` для ожидаемых 402/404: это нормальные результаты failure case.

## Шаг 1. Запустить окружение

### Бизнес-контекст

Клиенту нужна завершённая доставка. Чтобы проследить заказ, должны работать владельцы состояний, платежей и исполнения.

### Что делаем

Из корня репозитория запускаем локальное distributed environment: независимые процессы взаимодействуют через сеть, а не вызовы функций в одной программе.

### Команда / действие

```sh
make up
make ps
make health
```

### Что ожидаем увидеть

Три HTTP health, PostgreSQL для каждого владельца, Kafka topics, Collector, Prometheus, Grafana, Loki и Jaeger: OK. kafka-init завершается exit 0; это одноразовый initializer, а не бизнес-сервис.

### Где посмотреть в коде

[compose.yaml](compose.yaml); [services/order/cmd/order/main.go](services/order/cmd/order/main.go) — `run`.

### Что этот шаг демонстрирует

Startup dependencies, healthchecks, автоматические migrations и Composition Root.

### Стартовая проверка

До занятия убедитесь, что `make demo && make observe` проходит. Для ручного показа начните с `make reset && make up`; reset удалит demo-данные. Оставьте один terminal с variables из последующих шагов.

## Шаг 2. Начать с User Story

### Бизнес-контекст

Клиент хочет выбрать еду, оплатить её, дождаться приготовления и доставки.

### Что делаем

Читаем бизнесовое описание, вслух проверяем GIVEN/WHEN/THEN.

### Команда / действие

```sh
cat docs/user-story.md
```

### Что ожидаем увидеть

Happy и failure acceptance criteria; состояния Created, Paid, Accepted, Cooking, Ready, Delivering, Completed.

### Где посмотреть в коде

[docs/user-story.md](docs/user-story.md); [services/order/internal/domain/order.go](services/order/internal/domain/order.go) — `Total, StatusForSequence`.

### Что этот шаг демонстрирует

Бизнес-задача предшествует выбору транспорта и технологий.

### Вопрос студентам

Что должен получить клиент сразу, а что может произойти после ответа HTTP? Какое действие запрещено при отказе оплаты?

## Шаг 3. Посмотреть C4 Context

### Бизнес-контекст

Нужно понять систему глазами клиента без разбора Go-кода.

### Что делаем

Открываем Markdown с готовой диаграммой и исходным PlantUML; читаем Customer → GoDelivery, затем значимые инфраструктурные зависимости.

### Команда / действие

Откройте [docs/architecture/01-context.md](docs/architecture/01-context.md).

### Что ожидаем увидеть

Одна система, Customer и среды persistence/events/observability. Нет четвёртого business service.

### Где посмотреть в коде

[docs/architecture/01-context.md](docs/architecture/01-context.md).

### Что этот шаг демонстрирует

Различие business context и технических компонентов.

## Шаг 4. Посмотреть C4 Container/Component

### Бизнес-контекст

Решение оплаты и выполнение доставки меняются по разным правилам.

### Что делаем

Разбираем три service boundaries, три базы, REST и два event потока. Затем показываем компоненты внутри каждой границы.

### Команда / действие

Откройте [02-containers.md](docs/architecture/02-containers.md), [03-components.md](docs/architecture/03-components.md) и [04-sequence.md](docs/architecture/04-sequence.md).

### Что ожидаем увидеть

Order → Payment по REST; Order → Kafka → Fulfillment и обратные status events. Domain/usecase/adapters/delivery видны отдельно.

### Где посмотреть в коде

[services/order/internal/usecase/order.go](services/order/internal/usecase/order.go) — `Store, Payments, New`. [services/fulfillment/internal/usecase/fulfillment.go](services/fulfillment/internal/usecase/fulfillment.go) — `Store`.

### Что этот шаг демонстрирует

Service boundary, database ownership, Dependency Inversion и явный graph в main. Kitchen + Delivery объединены для обучения; production может разделять эти contexts.

## Шаг 5. Посмотреть Swagger

### Бизнес-контекст

Нужны публичные способы выбрать меню, оплатить и узнать состояние.

### Что делаем

Открываем все три Swagger UI; сопоставляем responsibility и endpoints.

### Команда / действие

[Order](http://localhost:8081/swagger/), [Payment](http://localhost:8082/swagger/), [Fulfillment](http://localhost:8083/swagger/). Нажмите Try it out у GET menu.

### Что ожидаем увидеть

У каждого свой contract, request/response models, errors/status codes/examples. Assets загружаются локально.

### Где посмотреть в коде

[services/order/api/openapi.yaml](services/order/api/openapi.yaml). [services/payment/api/openapi.yaml](services/payment/api/openapi.yaml). [services/fulfillment/api/openapi.yaml](services/fulfillment/api/openapi.yaml).

### Что этот шаг демонстрирует

API как transport contract. Fulfillment advance — технический demo API, а не интерфейс клиента доставки.


## Шаг 6. Получить меню

### Бизнес-контекст

Клиент выбирает существующее блюдо ресторана.

### Что делаем

Выполняем GET, сверяем цены и currency.

### Команда / действие

```sh
curl -sS http://localhost:8081/api/v1/menu
```

### Что ожидаем увидеть

restaurant-1, RUB, Маргарита 700 ₽ и Пепперони 900 ₽. Меню детерминировано.

### Где посмотреть в коде

[services/order/internal/domain/order.go](services/order/internal/domain/order.go) — `Menu`. [services/order/internal/delivery/http.go](services/order/internal/delivery/http.go) — `menu`.

### Что этот шаг демонстрирует

Простой request/response и правило расчёта стоимости.


## Шаг 7. Создать заказ

### Бизнес-контекст

Клиент выбрал две Маргариты: ожидаем 1400 ₽ и успешную оплату.

### Что делаем

Выполняем настоящий curl. Сохраняем response/headers для дальнейшего исследования; helper только читает результат и не создаёт второй заказ.

### Команда / действие

```sh
mkdir -p .demo
curl -sS -D .demo/checkout.headers -o .demo/checkout.json \
  -X POST http://localhost:8081/api/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"customer-1","restaurant_id":"restaurant-1","items":[{"dish_id":"pizza-margherita","quantity":2}],"payment_token":"tok-success"}'
cat .demo/checkout.headers
cat .demo/checkout.json
docker compose run --rm --no-deps -T demo-tools remember
ORDER_ID=$(docker compose run --rm --no-deps -T demo-tools field order_id)
PAYMENT_ID=$(docker compose run --rm --no-deps -T demo-tools field payment_id)
TRACE_ID=$(docker compose run --rm --no-deps -T demo-tools field trace_id)
TRACEPARENT=$(docker compose run --rm --no-deps -T demo-tools field traceparent)
export ORDER_ID PAYMENT_ID TRACE_ID TRACEPARENT
```

### Что ожидаем увидеть

HTTP 201, уникальные id/payment_id, total 1400, status Paid, sequence 0 в checkout response. GET позже может показать Accepted — consumer работает асинхронно. X-Trace-ID и X-Workflow-Traceparent присутствуют.

### Где посмотреть в коде

[services/order/internal/delivery/http.go](services/order/internal/delivery/http.go) — `create`. [services/order/internal/usecase/order.go](services/order/internal/usecase/order.go) — `Create`.

### Что этот шаг демонстрирует

Бизнес-правила → Created в SQL → synchronous Payment → Paid + outbox commit.

### Комментарий преподавателю

Оставьте переменные в этом terminal. Альтернативная автоматизированная команда для того же шага — `make checkout`; после неё field commands работают аналогично. Не выполняйте её дополнительно к curl без намерения создать ещё один заказ.

## Шаг 8. Найти REST-взаимодействие

### Бизнес-контекст

Нельзя начать приготовление без известного успешного результата оплаты.

### Что делаем

Открываем PaymentClient.Pay; показываем JSON DTO, request context, 2s timeout, traceparent и mapping HTTP status.

### Команда / действие

В editor откройте [services/order/internal/repository/payment_client.go](services/order/internal/repository/payment_client.go) — `Pay`. Затем в Jaeger (шаг 18) найдите тот же вызов по TRACE_ID.

### Что ожидаем увидеть

В коде видны POST /payments, client span, WithContext и отказы 402/503. Order не получает SQL connection другого сервиса.

### Где посмотреть в коде

[services/order/internal/repository/payment_client.go](services/order/internal/repository/payment_client.go) — `NewPaymentClient, Pay`. [services/order/cmd/order/main.go](services/order/cmd/order/main.go) — `run`.

### Что этот шаг демонстрирует

Временное связывание: checkout ждёт Payment. Timeout ограничивает ожидание; cancellation ограничивает request lifetime.

## Шаг 9. Проверить Payment DB

### Бизнес-контекст

Оплата — состояние отдельного владельца.

### Что делаем

Смотрим сохранённый Payment, сопоставляем order_id и payment_id.

### Команда / действие

```sh
make db-payments
curl -sS "http://localhost:8082/api/v1/payments/$PAYMENT_ID"
```

### Что ожидаем увидеть

Одна строка Succeeded, order_id совпадает, total 1400. Token отсутствует в таблице.

### Где посмотреть в коде

[services/payment/internal/repository/postgres.go](services/payment/internal/repository/postgres.go) — `Save`. [services/payment/migrations/000001_init.up.sql](services/payment/migrations/000001_init.up.sql).

### Что этот шаг демонстрирует

Immutable payment decision, unique order_id и идемпотентность повторного payment запроса.

### Вопрос студентам

Почему Payment использует unique order_id? Что произойдёт при повторе с другой total?

## Шаг 10. Проверить Order DB

### Бизнес-контекст

Клиенту нужен статус своего заказа и связь с успешной оплатой.

### Что делаем

Смотрим orders и локальную outbox; показываем transactional boundary.

### Команда / действие

```sh
make db-orders
```

### Что ожидаем увидеть

Paid либо Accepted (зависит от момента), payment_id сохранён. outbox содержит paid DTO; published_at станет непустым после ack Kafka.

### Где посмотреть в коде

[services/order/internal/repository/postgres.go](services/order/internal/repository/postgres.go) — `MarkPaid, enqueue`. [services/order/migrations/README.md](services/order/migrations/README.md).

### Что этот шаг демонстрирует

ACID только в собственной БД; Paid и намерение publish сохраняются вместе. Schema версионируется go-migrate; CREATE/INDEX — DDL, INSERT/UPDATE/SELECT — DML.

## Шаг 11. Посмотреть Kafka

### Бизнес-контекст

Оплаченный заказ должен перейти к приготовлению после завершения checkout.

### Что делаем

Делаем конечный snapshot обоих topics; найдите paid event с key = ORDER_ID.

### Команда / действие

```sh
make kafka-events
```

### Что ожидаем увидеть

topic order.paid.v1, partition, offset, key, payload, event_id/type/version, order_id/payment_id/total и traceparent в headers. Snapshot не меняет consumer group offsets.

### Где посмотреть в коде

[contracts/events/order-paid-v1.md](contracts/events/order-paid-v1.md). [services/order/internal/repository/outbox.go](services/order/internal/repository/outbox.go) — `flush`. [internal/platform/bus/bus.go](internal/platform/bus/bus.go) — `Publish`.

### Что этот шаг демонстрирует

Producer, broker, отдельный event transport DTO и propagation вне JSON payload.

## Шаг 12. Посмотреть Consumer

### Бизнес-контекст

Кухня должна принять paid event без повторного создания Fulfillment при redelivery.

### Что делаем

Читаем Fulfillment delivery mapping и последовательный Kafka loop; смотрим group offsets.

### Команда / действие

```sh
make kafka-groups
```

### Что ожидаем увидеть

fulfillment-paid-v1 и order-status-v1; CURRENT-OFFSET — следующая запись после committed message. При успешном workflow lag стремится к 0.

### Где посмотреть в коде

[services/fulfillment/internal/delivery/kafka.go](services/fulfillment/internal/delivery/kafka.go) — `PaidConsumer`. [internal/platform/bus/bus.go](internal/platform/bus/bus.go) — `Run`. [services/fulfillment/internal/repository/postgres.go](services/fulfillment/internal/repository/postgres.go) — `Accept`.

### Что этот шаг демонстрирует

Fetch → local DB transaction → offset commit. При handler error повторяется та же запись, а не следующая.

## Шаг 13. Проверить Fulfillment DB

### Бизнес-контекст

Система начала исполнение оплаченного заказа.

### Что делаем

Ждём Accepted с deadline, затем смотрим API и собственную базу.

### Команда / действие

```sh
docker compose run --rm --no-deps -T demo-tools wait-status fulfillment Accepted
make db-fulfillment
curl -sS "http://localhost:8083/api/v1/fulfillments/$ORDER_ID"
```

### Что ожидаем увидеть

Один Fulfillment: id=order_id, Accepted, sequence=1; один processed paid event и status outbox Accepted.

### Где посмотреть в коде

[services/fulfillment/internal/usecase/fulfillment.go](services/fulfillment/internal/usecase/fulfillment.go) — `Accept`. [services/fulfillment/internal/repository/postgres.go](services/fulfillment/internal/repository/postgres.go) — `Accept`.

### Что этот шаг демонстрирует

Раздельное state ownership и async processing, deduplication в одной transaction.

## Шаг 14. Подвигать статусы

### Бизнес-контекст

Заказ нужно приготовить и доставить. Время процесса управляем вручную для наблюдения.

### Что делаем

Выполняем по одному POST. После каждого перехода выполните шаг 15, затем возвращайтесь к следующему POST ниже.

### Команда / действие

```sh
# Accepted -> Cooking
curl -sS -X POST "http://localhost:8083/api/v1/fulfillments/$ORDER_ID/advance" -H "traceparent: $TRACEPARENT"
docker compose run --rm --no-deps -T demo-tools wait-status order Cooking
# Cooking -> Ready
curl -sS -X POST "http://localhost:8083/api/v1/fulfillments/$ORDER_ID/advance" -H "traceparent: $TRACEPARENT"
docker compose run --rm --no-deps -T demo-tools wait-status order Ready
# Ready -> Delivering
curl -sS -X POST "http://localhost:8083/api/v1/fulfillments/$ORDER_ID/advance" -H "traceparent: $TRACEPARENT"
docker compose run --rm --no-deps -T demo-tools wait-status order Delivering
# Delivering -> Completed
curl -sS -X POST "http://localhost:8083/api/v1/fulfillments/$ORDER_ID/advance" -H "traceparent: $TRACEPARENT"
docker compose run --rm --no-deps -T demo-tools wait-status order Completed
```

### Что ожидаем увидеть

Четыре response: Cooking seq2, Ready seq3, Delivering seq4, Completed seq5. Пятый advance даёт 409.

### Где посмотреть в коде

[services/fulfillment/internal/domain/fulfillment.go](services/fulfillment/internal/domain/fulfillment.go) — `Next`. [services/fulfillment/internal/repository/postgres.go](services/fulfillment/internal/repository/postgres.go) — `Advance`.

### Что этот шаг демонстрирует

State transitions, SELECT FOR UPDATE и атомарная запись status/outbox. Header объединяет manual requests в исходный trace.

## Шаг 15. Смотреть обратные события

### Бизнес-контекст

Клиент читает статус из Order, хотя исполнением занимается Fulfillment.

### Что делаем

После каждого отдельного advance шага 14 показываем обратный event и updated Order projection.

### Команда / действие

```sh
make kafka-events
make db-orders
```

### Что ожидаем увидеть

Для ORDER_ID найдите fulfillment.status-changed.v1 с новым sequence/status. После пяти состояний есть Accepted, Cooking, Ready, Delivering, Completed; processed_events Order содержит их event IDs.

### Где посмотреть в коде

[contracts/events/fulfillment-status-changed-v1.md](contracts/events/fulfillment-status-changed-v1.md). [services/order/internal/repository/postgres.go](services/order/internal/repository/postgres.go) — `ApplyStatus`.

### Что этот шаг демонстрирует

Eventually consistent read model, order_id partition ordering, deduplication и monotonic sequence.

## Шаг 16. Проверить Order

### Бизнес-контекст

Клиент должен увидеть доставленный заказ.

### Что делаем

Читаем Order REST и SQL.

### Команда / действие

```sh
curl -sS "http://localhost:8081/api/v1/orders/$ORDER_ID"
make db-orders
make db-fulfillment
```

### Что ожидаем увидеть

Order Completed sequence5; Fulfillment Completed sequence5. Данные согласованы событиями, а не cross-service JOIN.

### Где посмотреть в коде

[services/order/internal/delivery/http.go](services/order/internal/delivery/http.go) — `get`. [services/order/internal/usecase/order.go](services/order/internal/usecase/order.go) — `ApplyStatus`.

### Что этот шаг демонстрирует

Acceptance criteria исполнены; write owner и read owner обновляют только собственные БД.

## Шаг 17. Structured Logs

### Бизнес-контекст

Нужно объяснить, кто и когда оплатил и обработал конкретный заказ.

### Что делаем

Открываем Grafana Explore, datasource Loki, Last 15 minutes.

### Команда / действие

[Grafana Explore](http://localhost:3000/explore). Подставьте ORDER_ID в запрос:

```logql
{service_name=~".+"} | json | order_id="ORDER_ID"
```

Для trace:

```logql
{service_name=~".+"} | json | trace_id="TRACE_ID"
```

### Что ожидаем увидеть

JSON поля timestamp/level/service/operation/order_id/payment_id/trace_id/span_id; записи checkout, authorize, publish, accept, advance, apply_status, commit. В trace_id derived link открывает Jaeger.

### Где посмотреть в коде

[internal/platform/telemetry/telemetry.go](internal/platform/telemetry/telemetry.go) — `jsonOTLPHandler.Handle`. [deploy/grafana/provisioning/datasources/datasources.yaml](deploy/grafana/provisioning/datasources/datasources.yaml).

### Что этот шаг демонстрирует

Structured logs и correlation. Поля есть там, где полезны; token/пароли не попадают в telemetry.

## Шаг 18. Открыть Distributed Trace

### Бизнес-контекст

Нужно увидеть выполнение одного workflow через границы процессов.

### Что делаем

Открываем точный trace после задержки экспорта 2–10s.

### Команда / действие

```sh
printf "http://localhost:16686/trace/%s\n" "$TRACE_ID"
```
Откройте напечатанный URL. Альтернатива: Jaeger Search → Service order-service → Operation POST /api/v1/orders → Find Traces.

### Что ожидаем увидеть

Один trace с Order request/usecase, SQL, Payment client/server/SQL, Kafka producer/consumer и Fulfillment SQL. При manual headers также видны все advance/status events.

### Где посмотреть в коде

[internal/platform/web/web.go](internal/platform/web/web.go) — `Instrument`. [internal/platform/bus/bus.go](internal/platform/bus/bus.go) — `Publish, Run`. [services/order/internal/repository/payment_client.go](services/order/internal/repository/payment_client.go) — `Pay`.

### Что этот шаг демонстрирует

HTTP и Kafka W3C propagation; parent relationships продолжаются после исходного response.

### Комментарий преподавателю

Jaeger UI http://localhost:16686 — основной trace viewer; Grafana Jaeger datasource обеспечивает logs shortcuts. Если manual advance выполнен без traceparent, найдите его отдельный trace по Fulfillment service; order_id связывает записи.

## Шаг 19. Найти DB Span

### Бизнес-контекст

База может быть причиной медленного checkout или consumer.

### Что делаем

В открытом trace разворачиваем SQL INSERT payments или SQL UPDATE orders.

### Команда / действие

Выберите SQL span; откройте Tags/Attributes. Сравните его duration с родительским HTTP/usecase span.

### Что ожидаем увидеть

db.system=postgresql, db.operation, db.collection/table, db.namespace и duration. Нет SQL parameter values, credentials и token.

### Где посмотреть в коде

[internal/platform/telemetry/telemetry.go](internal/platform/telemetry/telemetry.go) — `DB`. [services/payment/internal/repository/postgres.go](services/payment/internal/repository/postgres.go) — `Save`.

### Что этот шаг демонстрирует

SQL operation отдельным client span, DB duration и безопасные attributes.

## Шаг 20. Technical Dashboard

### Бизнес-контекст

Нужно оценить скорость и ошибки компонентов в совокупности.

### Что делаем

Генерируем traffic; затем открываем provisioned dashboard, фильтруем service/route.

### Команда / действие

```sh
make traffic
```
[Technical Dashboard](http://localhost:3000/d/godelivery-technical), Last 15 minutes, Refresh 5s.

### Что ожидаем увидеть

RPS, 4xx/5xx error rate, p50/p95/p99, top slow endpoints, handler errors, DB p95, Kafka throughput/consumer errors. route содержит /api/v1/orders/{id}, а не ID.

### Где посмотреть в коде

[deploy/grafana/dashboards/technical.json](deploy/grafana/dashboards/technical.json). [internal/platform/web/web.go](internal/platform/web/web.go) — `Instrument`.

### Что этот шаг демонстрирует

Low-cardinality aggregation. Rate требует нескольких scrape samples; consumer errors=0 при нормальной работе.

## Шаг 21. Business Dashboard

### Бизнес-контекст

Нужно понять, сколько заказов создано, оплачено, доставлено или неуспешно.

### Что делаем

Сопоставляем бизнесовые counters со snapshots базы и с HTTP metrics.

### Команда / действие

[Business Dashboard](http://localhost:3000/d/godelivery-business). Откройте Created/Paid/Completed/Failed, Successful/Failed Payments, Orders by status и processing duration.

### Что ожидаем увидеть

После traffic все соответствующие counters меняются; orders_by_status отражает текущий DB snapshot. Processing duration измеряется при первом переходе Order в Completed.

### Где посмотреть в коде

[deploy/grafana/dashboards/business.json](deploy/grafana/dashboards/business.json). [services/order/internal/usecase/order.go](services/order/internal/usecase/order.go) — `Create, ApplyStatus`.

### Что этот шаг демонстрирует

Business metric отражает бизнес-факт, technical metric — выполнение компонента. Counters сбрасываются при рестарте; DB gauge сохраняет смысл.

## Шаг 22. Traffic Map

### Бизнес-контекст

Хотим увидеть реальные runtime relationships, а не предполагаемую схему.

### Что делаем

Открываем Node Graph после traffic, затем связанные rate/error/latency panels.

### Команда / действие

[Runtime Traffic Map](http://localhost:3000/d/godelivery-traffic), Last 15 minutes.

### Что ожидаем увидеть

Order → Payment; Order → Kafka → Fulfillment; Fulfillment → Kafka → Order. Рёбра получены из service_calls_total реальных adapters, без статической topology. На ребре счёт вызовов/сообщений за выбранный диапазон.

### Где посмотреть в коде

[deploy/grafana/dashboards/traffic.json](deploy/grafana/dashboards/traffic.json). [internal/platform/telemetry/telemetry.go](internal/platform/telemetry/telemetry.go) — `Call`. [docs/observability.md](docs/observability.md).

### Что этот шаг демонстрирует

C4 описывает границы/намерение, runtime map — фактический traffic. Kafka edges логические: producer/ack и consumer/commit, не broker internal hops.

## Шаг 23. Payment Failure

### Бизнес-контекст

Провайдер отказал: отправлять заказ на кухню нельзя.

### Что делаем

Повторяем checkout с deterministic token tok-decline; сохраняем отдельный failed response.

### Команда / действие

```sh
curl -sS -D .demo/checkout-failed.headers -o .demo/checkout-failed.json \
  -X POST http://localhost:8081/api/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"customer-1","restaurant_id":"restaurant-1","items":[{"dish_id":"pizza-pepperoni","quantity":1}],"payment_token":"tok-decline"}'
cat .demo/checkout-failed.headers
cat .demo/checkout-failed.json
docker compose run --rm --no-deps -T demo-tools remember-failure
FAILED_ORDER_ID=$(docker compose run --rm --no-deps -T demo-tools field failed_order_id)
FAILED_TRACE_ID=$(docker compose run --rm --no-deps -T demo-tools field failed_trace_id)
export FAILED_ORDER_ID FAILED_TRACE_ID
make db-payments
make db-orders
curl -sS "http://localhost:8081/api/v1/orders/$FAILED_ORDER_ID"
printf "http://localhost:16686/trace/%s\n" "$FAILED_TRACE_ID"
```

### Что ожидаем увидеть

HTTP 402; response.order.status=PaymentDeclined, Payment Declined. Order не Paid. В Loki error записи authorize/checkout; в trace Payment server/client и Order помечены error. payments_failed_total/orders_failed_total растут.

### Где посмотреть в коде

[services/payment/internal/domain/payment.go](services/payment/internal/domain/payment.go) — `Decide`. [services/order/internal/usecase/order.go](services/order/internal/usecase/order.go) — `Create`.

### Что этот шаг демонстрирует

Business failure mapping и сигнал диагностики на каждом слое.

### Комментарий преподавателю

При желании воспроизведите tok-slow аналогичным запросом: ожидайте 503 и PaymentUnknown. Эта дополнительная проверка не заменяет decline; возможную оплату нужно сверять, а не считать автоматически declined.

## Шаг 24. Доказать отсутствие Event и Fulfillment

### Бизнес-контекст

Недостаточно увидеть error response: нужно доказать, что side effects не запущены.

### Что делаем

Проверяем paid topic во всех partitions, outbox и Fulfillment таблицу/API.

### Команда / действие

```sh
make verify-failure
make kafka-events
curl -sS -i "http://localhost:8083/api/v1/fulfillments/$FAILED_ORDER_ID"
make db-fulfillment
```

### Что ожидаем увидеть

verify-failure exit0; нет paid outbox для failed_order_id; ни одного paid Kafka message с этим key; Fulfillment API 404 и ни одной записи в его БД. Существующие чужие events не считаем ошибкой.

### Где посмотреть в коде

[cmd/demo/main.go](cmd/demo/main.go) — `verifyFailure, messages`. [services/order/internal/usecase/order.go](services/order/internal/usecase/order.go) — `Create`.

### Что этот шаг демонстрирует

Проверка отрицательного acceptance criterion через durable state и ограниченный snapshot Kafka.

## Шаг 25. Расследовать: заказ не начал готовиться

### Бизнес-контекст

Пользователь сообщает: «заказ создан, но не готовится».

### Что делаем

Строим расследование от symptom до причины; используем failed_order_id, потом сравниваем со completed.

### Команда / действие

1. `curl -sS "http://localhost:8081/api/v1/orders/$FAILED_ORDER_ID"`.
2. `make db-orders`: status/payment_id/outbox.
3. Loki: `{service_name=~".+"} | json | order_id="FAILED_ORDER_ID"`.
4. Открыть напечатанный FAILED_TRACE_ID: HTTP status/error и SQL.
5. `make db-payments`: Declined.
6. `make verify-failure`, `make kafka-events`, `make kafka-groups`.
7. `make db-fulfillment`: запись отсутствует.

После обсуждения автоматизированное повторение всего сценария:

```sh
make demo
make observe
make test
make test-integration
```

### Что ожидаем увидеть

Причина — подтверждённый payment decline; Fulfillment отсутствие здесь ожидаемо. Если API Paid, но Fulfillment отсутствует: ищите pending outbox, broker availability, consumer error/group lag/processed_events. Если Accepted, но Cooking нет: demo ожидает advance.

### Где посмотреть в коде

[docs/design.md](docs/design.md). [docs/observability.md](docs/observability.md). [internal/platform/bus/bus.go](internal/platform/bus/bus.go) — `Run`.

### Что этот шаг демонстрирует

API → Order DB → Logs → Trace → Payment → Kafka → Fulfillment. Диагностика объясняет причинную связь и различает expected business refusal/technical failure.
