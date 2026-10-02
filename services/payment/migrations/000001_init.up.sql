CREATE TABLE payments (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    total BIGINT NOT NULL CHECK(total > 0),
    status TEXT NOT NULL CHECK(status IN ('Succeeded','Declined')),
    created_at TIMESTAMPTZ NOT NULL
);
