BEGIN;

CREATE INDEX IF NOT EXISTS idx_account_symbol_spec_reported_at
	ON echo.account_symbol_spec (account_id, reported_at_ms DESC);

CREATE INDEX IF NOT EXISTS idx_account_symbol_spec_updated_at
	ON echo.account_symbol_spec (updated_at DESC);

COMMIT;

