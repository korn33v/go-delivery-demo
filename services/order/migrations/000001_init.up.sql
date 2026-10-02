CREATE TABLE orders (
    id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,
    restaurant_id TEXT NOT NULL,
    items JSONB NOT NULL,
    total BIGINT NOT NULL CHECK(total > 0),
    status TEXT NOT NULL CHECK(status IN ('Created','Paid','Accepted','Cooking','Ready','Delivering','Completed','PaymentDeclined','PaymentUnknown')),
    payment_id TEXT NOT NULL DEFAULT '',
    sequence INTEGER NOT NULL DEFAULT 0 CHECK(sequence BETWEEN 0 AND 5),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX orders_status_idx ON orders(status);
CREATE TABLE processed_events (
    event_id TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE outbox (
    event_id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    message_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    trace_headers JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    published_at TIMESTAMPTZ
);
CREATE INDEX outbox_pending_idx ON outbox(created_at) WHERE published_at IS NULL;
