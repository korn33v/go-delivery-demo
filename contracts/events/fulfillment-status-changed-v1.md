# fulfillment.status-changed.v1

Producer: Fulfillment outbox. Consumer: Order, group `order-status-v1`. Topic = event type, 3 partitions, key = order_id. Headers: event_id, traceparent, необязательный tracestate.

```json
{"event_id":"event-status-demo","event_type":"fulfillment.status-changed.v1","event_version":1,"occurred_at":"2026-10-02T12:00:05Z","order_id":"order-demo","fulfillment_id":"order-demo","status":"Cooking","sequence":2}
```

Status / sequence: Accepted=1, Cooking=2, Ready=3, Delivering=4, Completed=5. Event ID защищает от повторной обработки; monotonic sequence защищает от регрессии старым event с иным ID. Пропуск sequence вызывает consumer error и retry; successful offset commit наступает только после SQL transaction commit.

Version отвечает за совместимость, event_id — за идентичность бизнес-факта, message key — за partition routing/order, order_id — за связь с заказом. Один key не делает consumer идемпотентным: для этого нужна deduplication в собственном processed_events. Trace context хранится в Kafka headers, отдельно от бизнесового JSON contract.

Fulfillment.TraceHeaders — внутренняя metadata хранения/демонстрации, не часть event payload. Структуры DTO находятся в contracts/events/events.go. Совместимое добавление optional поля допустимо без смены v1; переименование/изменение значения поля требует согласованной новой версии consumer/producer.
