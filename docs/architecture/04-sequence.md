# Sequence: happy path и commit boundary

![Sequence](rendered/04-sequence.svg)

HTTP 201 закрывает checkout, но не доставку. Статус приходит обратно асинхронно. В каждом consumer commit offset следует за SQL commit. Header traceparent идёт через HTTP, outbox metadata и Kafka headers.

```plantuml
@startuml
actor Client
participant "Order Service" as Order
database "Order DB" as ODB
participant "Payment Service" as Payment
database "Payment DB" as PDB
queue Kafka
participant "Fulfillment Service" as F
database "Fulfillment DB" as FDB
Client -> Order: GET /api/v1/menu (HTTP)
Order --> Client: 200 menu
Client -> Order: POST /api/v1/orders (HTTP JSON)
Order -> ODB: SQL INSERT Created
Order -> Payment: POST /api/v1/payments + traceparent
Payment -> PDB: SQL INSERT Succeeded (unique order_id)
Payment --> Order: 201 payment
Order -> ODB: BEGIN; UPDATE Paid; INSERT paid outbox; COMMIT
Order --> Client: 201 Paid + X-Trace-ID
Order -> Kafka: publish order.paid.v1 + W3C headers
Order -> ODB: SQL UPDATE outbox published_at
Kafka -> F: consume paid (group fulfillment-paid-v1)
F -> FDB: BEGIN; INSERT processed_events; INSERT Accepted;
F -> FDB: INSERT status outbox; COMMIT
F -> Kafka: commit paid offset
F -> Kafka: publish status Accepted (sequence 1)
Kafka -> Order: consume status (group order-status-v1)
Order -> ODB: BEGIN; dedup; UPDATE Accepted; COMMIT
Order -> Kafka: commit status offset
loop Cooking / Ready / Delivering / Completed
 Client -> F: POST /fulfillments/{id}/advance + traceparent
 F -> FDB: BEGIN; SELECT FOR UPDATE; UPDATE status;
 F -> FDB: INSERT status outbox; COMMIT
 F --> Client: 200 next status
 F -> Kafka: publish fulfillment.status-changed.v1
 Kafka -> Order: consume status
 Order -> ODB: BEGIN; dedup + monotonic UPDATE; COMMIT
 Order -> Kafka: commit offset
end
Client -> Order: GET /api/v1/orders/{id}
Order -> ODB: SQL SELECT
Order --> Client: 200 Completed
@enduml
```
