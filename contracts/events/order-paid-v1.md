# order.paid.v1

Producer: Order outbox. Consumer: Fulfillment, group `fulfillment-paid-v1`. Topic = event type, 3 partitions, key = order_id, delivery at-least-once. Headers: event_id, traceparent, необязательный tracestate.

```json
{"event_id":"event-demo","event_type":"order.paid.v1","event_version":1,"occurred_at":"2026-10-02T12:00:00Z","order_id":"order-demo","payment_id":"payment-demo","total":1400}
```

| Поле | Тип / смысл |
|---|---|
| event_id | Непустой уникальный ID одного события; при outbox retry неизменен |
| event_type / event_version | Версия transport contract; несовместимое изменение получает новый topic/type/version |
| occurred_at | UTC RFC3339 время бизнес-факта |
| order_id | Заказ и message key; partition ordering для одного заказа |
| payment_id | Доказанная успешная оплата; непустой |
| total | Положительное целое количество рублей |

Domain Order имеет items/customer/status/timestamps и меняется вместе с приложением. Событие содержит минимальный публичный факт; payment_token не передаётся. В contracts/events/events.go отдельный DTO, а не json.Marshal(domain.Order). Consumer проверяет type/version/key и сохраняет event_id с бизнес-состоянием в одном transaction. Invalid envelope вызывает error и не commit'ится.
