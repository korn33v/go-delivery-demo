# Как наблюдать workflow

## Signals

| Сигнал | Что отвечает | Пример |
|---|---|---|
| Structured log | Какое дискретное событие произошло? | payment declined с order_id, payment_id, trace_id |
| Technical metric | Как ведут себя компоненты в совокупности? | HTTP p95, DB latency, Kafka consumer errors |
| Business metric | Что произошло с бизнес-процессом? | orders_paid_total, orders_completed_total |
| Trace | Где и сколько времени выполнялся конкретный workflow? | checkout → Payment → SQL → Kafka → Fulfillment |

`slog` пишет JSON одновременно в stdout и в OTLP Logs. Collector отправляет body/metadata в Loki native OTLP endpoint. Индексный label — service_name; IDs остаются JSON/structured metadata. Dashboards используют низкую cardinality: service/route/status, operation/table, topic/group, source/target/protocol/outcome. ID запрещены в Prometheus labels; trace attributes и log fields их могут содержать.

## Готовые LogQL

Grafana → Explore → Loki; диапазон Last 15 minutes:

```logql
{service_name=~".+"} | json | order_id="ORDER_ID"
{service_name=~".+"} | json | trace_id="TRACE_ID"
{service_name="payment-service"} | json | level="ERROR"
{service_name="fulfillment-service"} | json | operation="kafka.commit"
```

Замените ORDER_ID/TRACE_ID реальными значениями. В JSON-записи поле trace_id имеет derived link `Open in Jaeger`. В Jaeger datasource trace → span Logs использует готовый tracesToLogsV2 query. Самостоятельный Loki поиск по trace_id также приведён в методичке и работает независимо от UI shortcut.

## PromQL

```promql
sum by(service)(rate(http_server_requests_total[2m]))
100 * sum(rate(http_server_requests_total{status=~"4..|5.."}[2m])) / clamp_min(sum(rate(http_server_requests_total[2m])), 0.001)
histogram_quantile(0.95, sum by(le,service)(rate(http_server_request_duration_seconds_bucket[2m])))
histogram_quantile(0.95, sum by(le,table,operation)(rate(db_operation_duration_seconds_bucket[2m])))
sum by(service,topic)(rate(kafka_messages_consumed_total[2m]))
sum(orders_created_total)
sum(orders_paid_total)
sum(orders_completed_total)
sum by(status)(orders_by_status)
```

Technical Dashboard имеет фильтры service/route и p50/p95/p99, top slow endpoints, handler errors, DB latency, Kafka throughput/errors. `make traffic` даёт достаточно samples для rate. HTTP/DB buckets покрывают миллисекунды и секунды; business duration имеет отдельные buckets до четырёх часов для ручного семинара. Percentile — оценка по buckets; демонстрационная нагрузка мала и не является benchmark.

## Distributed tracing

OTLP/HTTP идёт в Collector, затем Jaeger. Для HTTP W3C traceparent/tracestate передаются заголовками. Для Kafka те же значения находятся в message headers. Outbox сохраняет trace context как инфраструктурную metadata отдельно от payload: worker продолжает исходный trace. Consumer извлекает context до application use case. SQL wrapper создаёт отдельный client span с db.system, db.operation, db.collection, db.namespace. Значения SQL-параметров, tokens и credentials не логируются.

`make demo` выводит точный `http://localhost:16686/trace/<trace_id>`. Trace содержит server span, Order checkout, SQL orders/outbox, HTTP Payment client/server, SQL payments, Kafka producer/consumer, Fulfillment SQL. В методичке дальнейшие advance передают исходный traceparent. В длинном trace spans могут добавляться после закрытия первоначального HTTP span: это нормально для async demo. Sampling = 100% только для локального обучения.

## Runtime Traffic Map

http://localhost:3000/d/godelivery-traffic

Adapter при фактическом вызове обновляет service_calls_total и service_call_duration_seconds: Order → Payment (HTTP), Order → Kafka, Kafka → Fulfillment, Fulfillment → Kafka, Kafka → Order. Prometheus строит id/source/target table из этих временных рядов; Grafana Node Graph выводит реальные observed edges. Счёт на ребре — вызовы/сообщения в выбранном диапазоне. Рядом — rate, error rate и p95. Счётчики регистрируются с нулевым baseline; фильтр `increase > 0` исключает такие рёбра до реального traffic. Статических рёбер и заранее заданных node lists в dashboard нет. Prometheus `rate/increase` требует минимум два scrape samples; `make traffic` даёт время и измерения для карты.

Учитываются логические взаимодействия, а не реальная сеть Kafka broker и не end-to-end latency доставки. Consumer latency — время обработки + commit, producer — время ack, REST — время HTTP round trip. Это ближайший корректный вариант для Kafka-aware карты простого стенда без service mesh. C4 описывает проектные границы и допустимые зависимости; Traffic Map показывает фактические вызовы, включая ошибочные.

## Доказательства

`make observe` после `make demo` делает реальные HTTP queries в Prometheus/Loki/Jaeger/Grafana и проверяет положительные business counters, trace трёх сервисов, SQL duration/attributes, Kafka producer/consumer spans, logs, provisioning, пять observed edges и Grafana dataframe для Node Graph. При отсутствии исходного scrape baseline запустите `make traffic` и повторите проверку. `make kafka-events` делает ограниченный snapshot без consumer group, не затрагивает business offsets. `make verify-failure` проверяет Declined в обеих БД, отсутствие outbox и event по key во всех partitions, HTTP 404 Fulfillment и отсутствие его записи в БД.
