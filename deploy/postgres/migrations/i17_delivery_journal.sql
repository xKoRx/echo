CREATE SCHEMA IF NOT EXISTS echo;

CREATE TABLE IF NOT EXISTS echo.delivery_journal (
    command_id UUID PRIMARY KEY,
    trade_id UUID NOT NULL,
    agent_id TEXT NOT NULL,
    target_account_id TEXT NOT NULL,
    command_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    stage SMALLINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NULL,
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_delivery_journal_status_retry
    ON echo.delivery_journal (status, next_retry_at);

CREATE INDEX IF NOT EXISTS idx_delivery_journal_trade
    ON echo.delivery_journal (trade_id);

CREATE TABLE IF NOT EXISTS echo.delivery_retry_event (
    id BIGSERIAL PRIMARY KEY,
    command_id UUID NOT NULL REFERENCES echo.delivery_journal(command_id) ON DELETE CASCADE,
    stage SMALLINT NOT NULL,
    result SMALLINT NOT NULL,
    attempt INT NOT NULL,
    error TEXT NULL,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_delivery_retry_event_cmd
    ON echo.delivery_retry_event (command_id, created_at DESC);

