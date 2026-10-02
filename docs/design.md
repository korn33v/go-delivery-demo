# Принятые решения

## Границы и владение

Order знает меню и собственный checkout/status. Payment принимает решение об оплате и не знает приготовление. Fulfillment получает только оплаченные заказы; Kitchen/Delivery здесь сознательно объединены, хотя production bounded contexts могут разделяться. Ни один сервис не импортирует `internal` другого: отдельные Go internal boundaries проверяются компилятором. Один корневой Go module упрощает сборку; deployment units остаются независимыми.

Каждая PostgreSQL-роль имеет CONNECT только к собственной базе. Общие инфраструктурные helpers (`bus`, `web`, `telemetry`, `app`) не содержат бизнес-таблиц или общего repository. SQL каждого сервиса находится в его repository. `domain` не импортирует HTTP, SQL/Kafka/OTel SDK.

## Синхронная граница

Order нужен результат Payment, чтобы ответить клиенту и решить, можно ли начать Fulfillment. REST JSON клиент имеет 2s timeout и request context cancellation, передаёт W3C headers и проверяет response schema/status. Declined — HTTP 402, invalid business input — 422, missing resource — 404, completed advance — 409, неизвестный payment outcome — 503. JSON syntax — 400, неверный Content-Type — 415. Все API имеют собственный OpenAPI.

Сервис сохраняет Created до запроса оплаты. Нет распределённой ACID-транзакции между Order и Payment. В случае timeout сохраняется PaymentUnknown; сверка и компенсация платежей вне scope. Отказ гарантированно не создаёт paid event. Повторный **POST orders** создаёт новый заказ: глобального checkout idempotency key в demo нет. Payment идемпотентен по order_id, сумма неизменна.

## Асинхронная граница и outbox

Order транзакционно сохраняет Paid + transport DTO в своей outbox. Fulfillment аналогично сохраняет status + status DTO. Worker каждые 250ms читает неопубликованные записи по времени создания, синхронно пишет в Kafka с RequireAll и только затем отмечает published_at. При ошибке останавливает batch, чтобы не публиковать поздний sequence раньше раннего. При crash после publish, до UPDATE возможен повтор с прежним event_id. Это **at-least-once**, а не exactly-once.

Один worker на сервис достаточен для локального примера. Горизонтальный запуск нескольких outbox workers потребовал бы row claiming/locking; стенд его не демонстрирует. Consumer хранит processed_events и business state в одной локальной транзакции; rollback не помечает failed event обработанным. Затем commit'ит Kafka offset. Потерянный commit приводит к повтору, который не повторяет business effect.

Kafka topic имеет три partitions. Key = order_id; Kafka Hash balancer кладёт события одного заказа в одну partition. Sequence защищает Order от старых/повторных событий и выявляет пропуск. Между разными заказами глобального порядка нет.

Malformed message вызывает error, retry той же записи и отсутствие commit. Автоматического DLQ нет: это наглядное ограничение. Не commit'им poison message ради продолжения, потому что тогда ошибка стала бы молчаливой потерей. Consumer loop и retries прямолинейны и прокомментированы.

## Управляемый lifecycle и traces

Advance меняет ровно одно состояние под PostgreSQL FOR UPDATE. Fulfillment ID равен Order ID только для удобства демонстрации. Асинхронный ответ Order eventually consistent: curl сразу после advance может увидеть старый status. Demo ожидает следующий status с deadline, а не полагается на произвольный sleep.

HTTP response содержит X-Trace-ID и checkout X-Workflow-Traceparent. Передав этот traceparent в ручных advance, студент продолжает исходный distributed trace. `make demo` делает это автоматически. Без header каждый advance становится отдельным request trace; бизнес-связь по order_id остаётся. Это явный учебный выбор: долгие процессы в production часто используют span links/несколько traces и отдельный workflow ID.

## Constructor Injection

`main.go` каждого сервиса читает config, создаёт telemetry, применяет миграции, открывает database/sql, создаёт repository, Kafka adapters/REST client, use case и handlers. Ports находятся в usecase у потребителя. Студент видит весь graph в одном файле, без DI-framework. Context несёт lifetime/cancellation/trace context; tx передаётся repository helper явно.

## Границы observability

Traces Jaeger хранятся в памяти (до 100000 traces, с запасом на фоновый SQL polling во время занятия) и исчезают при перезапуске. PostgreSQL/Kafka/Loki/Prometheus используют Compose volumes. Business counters показывают события текущей жизни процессов, а не бухгалтерский исторический итог. `orders_by_status` регулярно читается из Order DB и отражает текущий снимок даже после рестарта. Ровно один экземпляр каждого сервиса исключает двойной подсчёт gauge.

Технические ошибки HTTP включают 4xx/5xx; declined — ожидаемый бизнес-отказ, но виден и в HTTP error rate. Карта связей собирается непосредственно на реальных REST/Kafka adapters; она измеряет логические producer/broker/consumer interactions. В неё не входят внутренние broker network hops, фоновые health polls и SQL связи. См. [observability.md](observability.md).
