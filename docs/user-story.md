# Business Context

Небольшой ресторан продаёт пиццу с доставкой. Клиенту нужен понятный ответ об оплате сразу; приготовление и доставка занимают время и могут идти после завершения HTTP-запроса. Нельзя отправлять неоплаченный заказ на кухню.

# User Story

Как клиент, я хочу выбрать блюдо, создать и оплатить заказ, дождаться приготовления и доставки и увидеть итоговый статус заказа.

# Acceptance Criteria

**GIVEN** клиент `customer-1` выбрал существующее блюдо ресторана `restaurant-1`, **WHEN** создаёт заказ с `tok-success`, **THEN** Order сохраняет заказ, **AND** рассчитывает сумму по меню, **AND** синхронно вызывает Payment по REST, **AND** Payment сохраняет `Succeeded`, **AND** Order сохраняет `Paid` и `order.paid.v1` в одной транзакции, **AND** outbox публикует событие в Kafka, **AND** Fulfillment получает его и сохраняет `Accepted`, **AND** каждый управляемый lifecycle transition публикует status event, **AND** после завершения Order становится `Completed`.

**GIVEN** Payment должен отклонить оплату (`tok-decline`), **WHEN** клиент создаёт заказ, **THEN** REST возвращает 402 и ID сохранённого заказа, **AND** Payment сохраняет `Declined`, **AND** Order сохраняет `PaymentDeclined`, **AND** заказ не становится Paid, **AND** ни paid outbox record, ни paid Kafka event не создаются, **AND** Fulfillment отсутствует, **AND** failure виден в логе, метрике и trace.

**GIVEN** consumer получает повторный event ID, **WHEN** обрабатывает сообщение, **THEN** не создаёт второй Fulfillment и не повторяет переход Order, **AND** безопасно подтверждает offset после commit.

# Happy Path

1. Menu: Маргарита 700 ₽, Пепперони 900 ₽.
2. Create: две Маргариты → total 1400 ₽ → Created (сохранён в БД).
3. Payment: `Succeeded` → Order `Paid` (HTTP 201).
4. Kafka: `order.paid.v1` → Fulfillment `Accepted`.
5. Четыре demo advance: `Cooking → Ready → Delivering → Completed`.
6. Для каждого из пяти состояний Fulfillment выходит status event; Order обновляет своё представление.

# Failure Case

`tok-decline` всегда даёт отказ без технической случайности. Для того же order_id Payment хранит прежний результат даже при повторном запросе с другим token. Order не читает payment_db: использует ответ API. `tok-slow` дополнительно задерживает авторизацию на 3 секунды при REST client timeout 2 секунды. Такой заказ имеет `PaymentUnknown`: timeout вообще не доказывает отсутствие списания в реальном провайдере. Сверка платежей намеренно вне scope.

# Out of Scope

Регистрация, OAuth/OIDC, реальный payment provider, GPS, courier dispatch, промокоды, Kubernetes, Service Mesh, API Gateway, frontend и сложная Saga. Kitchen и Delivery объединены только для обучения. Redis и gRPC в этой поставке не используются по явным требованиям текущей задачи.
