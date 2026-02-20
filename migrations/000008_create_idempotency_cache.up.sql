CREATE TABLE idempotency_cache (
    idempotency_key VARCHAR(255)  NOT NULL,
    user_id         UUID          NOT NULL,
    request_hash    VARCHAR(64)   NOT NULL,
    status_code     INT           NOT NULL,
    response_body   BYTEA         NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ   NOT NULL,
    PRIMARY KEY (idempotency_key, user_id)
);

CREATE INDEX idx_idempotency_cache_expires_at ON idempotency_cache (expires_at);
