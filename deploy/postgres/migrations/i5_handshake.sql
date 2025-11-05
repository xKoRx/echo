CREATE SCHEMA IF NOT EXISTS echo;

CREATE TABLE IF NOT EXISTS echo.account_symbol_registration_eval (
    evaluation_id UUID PRIMARY KEY,
    account_id TEXT NOT NULL,
    pipe_role TEXT NOT NULL,
    status TEXT NOT NULL,
    protocol_version INT NOT NULL,
    client_semver TEXT NOT NULL,
    global_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    global_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    required_features JSONB NOT NULL DEFAULT '[]'::jsonb,
    optional_features JSONB NOT NULL DEFAULT '[]'::jsonb,
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS echo.account_symbol_registration (
    evaluation_id UUID NOT NULL REFERENCES echo.account_symbol_registration_eval(evaluation_id) ON DELETE CASCADE,
    canonical_symbol TEXT NOT NULL,
    broker_symbol TEXT NOT NULL,
    status TEXT NOT NULL,
    warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    spec_age_ms BIGINT NOT NULL,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (evaluation_id, canonical_symbol)
);

CREATE INDEX IF NOT EXISTS idx_account_symbol_registration_eval_account
    ON echo.account_symbol_registration_eval (account_id, evaluated_at DESC);

CREATE OR REPLACE FUNCTION echo.notify_account_symbol_registration()
RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('echo_handshake_result', NEW.account_id || ':' || NEW.evaluation_id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_account_symbol_registration_eval_notify ON echo.account_symbol_registration_eval;
CREATE TRIGGER trg_account_symbol_registration_eval_notify
AFTER INSERT ON echo.account_symbol_registration_eval
FOR EACH ROW EXECUTE FUNCTION echo.notify_account_symbol_registration();
