COMPOSE ?= docker compose
TOOLS = $(COMPOSE) run --rm --no-deps demo-tools
.PHONY: help up down reset build test test-integration migrate-up migrate-down demo traffic logs ps kafka-events kafka-groups db-orders db-payments db-fulfillment health observe checkout advance failure verify-failure lint
help: ## Все команды проекта
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
up: ## Собрать, запустить, дождаться health всех компонентов
	mkdir -p .demo
	$(COMPOSE) up --build -d --wait --wait-timeout 480
	$(COMPOSE) build demo-tools
	$(TOOLS) health
down: ## Остановить окружение, сохранить volumes
	$(COMPOSE) down --remove-orphans
reset: ## Остановить и удалить ВСЕ demo-данные этого проекта
	mkdir -p .demo
	$(COMPOSE) --profile tools --profile test down --volumes --remove-orphans
	$(COMPOSE) run --rm --no-deps --entrypoint sh demo-tools -c 'rm -f /work/.demo/*.json /work/.demo/*.headers'
build: ## Собрать приложения и инструменты
	$(COMPOSE) --profile tools --profile test build
health: ## Проверить все HTTP endpoints, БД и Kafka
	$(TOOLS) health
test: ## Unit и HTTP integration tests (без внешних сервисов)
	$(COMPOSE) build tests
	$(COMPOSE) run --rm --no-deps tests go test -race ./...
lint: ## gofmt и go vet в контейнере
	$(COMPOSE) build tests
	$(COMPOSE) run --rm --no-deps tests sh -ec 'test -z "$$(gofmt -l internal services cmd contracts)"; go vet ./...'
test-integration: ## PostgreSQL/Kafka tests + E2E happy/failure + telemetry (после make up)
	$(COMPOSE) build tests demo-tools
	$(COMPOSE) run --rm --no-deps tests go test -race -tags=integration -p 1 ./...
	$(TOOLS) demo
	$(TOOLS) observe
migrate-up: ## Применить UP всех трёх сервисов
	$(COMPOSE) run --rm --no-deps order-service migrate up
	$(COMPOSE) run --rm --no-deps payment-service migrate up
	$(COMPOSE) run --rm --no-deps fulfillment-service migrate up
migrate-down: ## Удалить schema данных; выполнять на остановленных сервисах
	$(COMPOSE) stop order-service payment-service fulfillment-service
	$(COMPOSE) run --rm --no-deps order-service migrate down
	$(COMPOSE) run --rm --no-deps payment-service migrate down
	$(COMPOSE) run --rm --no-deps fulfillment-service migrate down
demo: ## Happy path + payment decline, проверки REST/SQL/Kafka
	$(TOOLS) demo
traffic: ## 30 сценариев с интервалом 2s (около 2 минут)
	sh scripts/generate-traffic.sh
observe: ## Проверить Loki/Prometheus/Jaeger/dashboards/runtime edges
	$(TOOLS) observe
checkout: ## Создать оплаченный заказ и сохранить его ID/trace для пошагового demo
	$(TOOLS) create
advance: ## Один переход текущего fulfillment
	$(TOOLS) advance
failure: ## Воспроизвести DECLINED и проверить отсутствие paid/fulfillment
	$(TOOLS) decline
verify-failure: ## Повторно доказать отсутствие event/outbox/fulfillment
	$(TOOLS) verify-failure
logs: ## Structured logs приложений и collector
	$(COMPOSE) logs -f --tail=100 order-service payment-service fulfillment-service otel-collector
ps: ## Состояние контейнеров
	$(COMPOSE) ps -a
kafka-events: ## Snapshot обоих topics: key, partition, offset, headers, payload
	$(TOOLS) inspect
kafka-groups: ## Consumer groups и committed offsets
	$(COMPOSE) exec -T kafka /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server kafka:9092 --describe --all-groups
db-orders: ## Данные Order DB, outbox и processed_events
	$(COMPOSE) exec -T postgres psql -U order_user -d order_db -c "SELECT id,status,total,payment_id,sequence,created_at FROM orders ORDER BY created_at;" -c "SELECT event_id,topic,message_key,published_at FROM outbox ORDER BY created_at;" -c "SELECT * FROM processed_events ORDER BY processed_at;"
db-payments: ## Данные Payment DB
	$(COMPOSE) exec -T postgres psql -U payment_user -d payment_db -c "SELECT id,order_id,total,status,created_at FROM payments ORDER BY created_at;"
db-fulfillment: ## Данные Fulfillment DB и outbox
	$(COMPOSE) exec -T postgres psql -U fulfillment_user -d fulfillment_db -c "SELECT id,order_id,status,sequence,created_at FROM fulfillments ORDER BY created_at;" -c "SELECT event_id,topic,message_key,published_at FROM outbox ORDER BY created_at;"
