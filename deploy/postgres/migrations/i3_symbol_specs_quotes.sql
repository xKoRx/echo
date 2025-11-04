-- i3: almacenamiento de especificaciones y snapshots de símbolos

CREATE TABLE IF NOT EXISTS echo.account_symbol_spec (
    account_id        TEXT    NOT NULL,
    canonical_symbol  TEXT    NOT NULL,
    broker_symbol     TEXT    NOT NULL,
    payload           JSONB   NOT NULL,
    reported_at_ms    BIGINT  NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, canonical_symbol)
);

CREATE INDEX IF NOT EXISTS idx_account_symbol_spec_account
    ON echo.account_symbol_spec (account_id);

CREATE TABLE IF NOT EXISTS echo.symbol_quote_latest (
    account_id       TEXT    NOT NULL,
    canonical_symbol TEXT    NOT NULL,
    broker_symbol    TEXT    NOT NULL,
    bid              NUMERIC(18,8) NOT NULL,
    ask              NUMERIC(18,8) NOT NULL,
    spread_points    NUMERIC(18,8) NOT NULL,
    timestamp_ms     BIGINT NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, canonical_symbol)
);

CREATE INDEX IF NOT EXISTS idx_symbol_quote_latest_timestamp
    ON echo.symbol_quote_latest (timestamp_ms DESC);


