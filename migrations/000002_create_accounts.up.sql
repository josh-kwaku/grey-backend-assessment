CREATE TABLE accounts (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID         NOT NULL REFERENCES users(id),
    currency        CHAR(3)      NOT NULL,
    account_type    VARCHAR(20)  NOT NULL DEFAULT 'user',

    balance         BIGINT       NOT NULL DEFAULT 0,
    version         BIGINT       NOT NULL DEFAULT 0,

    account_number  VARCHAR(50),
    routing_number  VARCHAR(50),
    iban            VARCHAR(34),
    swift_bic       VARCHAR(11),
    provider        VARCHAR(50),
    provider_ref    VARCHAR(255),

    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT chk_accounts_balance CHECK (balance >= 0)
);

CREATE UNIQUE INDEX idx_accounts_user_currency_type ON accounts (user_id, currency, account_type);
