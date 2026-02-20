CREATE TABLE payments (
    id                    UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key       VARCHAR(255)   NOT NULL,
    type                  VARCHAR(30)    NOT NULL,
    status                VARCHAR(30)    NOT NULL DEFAULT 'pending',

    source_account_id     UUID           NOT NULL REFERENCES accounts(id),

    dest_account_id       UUID           REFERENCES accounts(id),
    dest_account_number   VARCHAR(50),
    dest_iban             VARCHAR(34),
    dest_swift_bic        VARCHAR(11),
    dest_bank_name        VARCHAR(255),

    source_amount         BIGINT         NOT NULL,
    source_currency       CHAR(3)        NOT NULL,
    dest_amount           BIGINT         NOT NULL,
    dest_currency         CHAR(3)        NOT NULL,
    exchange_rate         DECIMAL(20,10),
    fee_amount            BIGINT         NOT NULL DEFAULT 0,
    fee_currency          CHAR(3),

    provider              VARCHAR(50),
    provider_ref          VARCHAR(255),
    failure_reason        TEXT,
    metadata              JSONB,

    created_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
    completed_at          TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_payments_idempotency_key ON payments (idempotency_key);
CREATE INDEX idx_payments_source_account ON payments (source_account_id);
CREATE INDEX idx_payments_dest_account ON payments (dest_account_id);
CREATE INDEX idx_payments_status ON payments (status);
