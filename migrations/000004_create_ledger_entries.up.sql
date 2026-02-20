CREATE TABLE ledger_entries (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id     UUID         NOT NULL REFERENCES payments(id),
    account_id     UUID         NOT NULL REFERENCES accounts(id),
    entry_type     VARCHAR(10)  NOT NULL,
    amount         BIGINT       NOT NULL,
    currency       CHAR(3)      NOT NULL,
    balance_before BIGINT       NOT NULL,
    balance_after  BIGINT       NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_entries_account ON ledger_entries (account_id);
CREATE INDEX idx_ledger_entries_payment ON ledger_entries (payment_id);
