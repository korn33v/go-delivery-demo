CREATE TABLE fulfillments (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK(status IN ('Accepted','Cooking','Ready','Delivering','Completed')),
    sequence INTEGER NOT NULL CHECK(sequence BETWEEN 1 AND 5),
    trace_headers JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
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
