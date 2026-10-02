# Migrations: fulfillment_db

Migration — versioned изменение schema. Код приложения и структура данных должны развёртываться совместимо: миграции хранятся рядом с кодом, а `schema_migrations` записывает version/dirty. В `main.go` go-migrate применяет UP перед запуском HTTP; `make up` не требует ручного SQL.

## UP / DOWN

`000001_init.up.sql` создаёт таблицы и constraints. `000001_init.down.sql` удаляет объекты в обратном порядке. `make migrate-up` применяет schema всех трёх сервисов. `make migrate-down` останавливает business services и удаляет schema/данные; перед повторным demo выполните `make reset && make up`, чтобы Kafka не переиграла старые сообщения в пустые таблицы.

## DDL: меняет структуру

- `CREATE TABLE fulfillments` — новая таблица.
- `CREATE INDEX` в Order/Fulfillment outbox — ускорение поиска pending events; unique constraint payments(order_id) создаёт собственный unique index.
- `ALTER TABLE fulfillments ADD COLUMN ...` — форма следующего versioned изменения; в текущем проекте лишней миграции с такой колонкой нет.
- `DROP TABLE fulfillments` — DOWN удаляет таблицу вместе с данными.

DDL определяет schema, constraints и indexes. Пример обсуждения будущей миграции (не выполнять на стенде): `ALTER TABLE fulfillments ADD COLUMN note TEXT;`.

## DML: работает с данными

Реальные adapters: `SELECT` читает состояние, `INSERT` сохраняет сущность. В Order/Fulfillment `UPDATE` меняет status; Payment immutable после решения. `DELETE FROM fulfillments WHERE id='...'` удалял бы одну строку без удаления таблицы — это пример DML, а не команда основного demo. Tests используют изолированные schemas и убирают их в cleanup.

DDL отвечает «какие таблицы/поля существуют», DML — «какие строки/значения в них сейчас». Не путайте `DELETE` с `DROP TABLE`. Каждый сервис применяет migrations только к своей БД; межсервисные foreign keys отсутствуют.
