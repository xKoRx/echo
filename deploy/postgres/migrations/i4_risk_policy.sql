BEGIN;

CREATE TABLE IF NOT EXISTS echo.account_strategy_risk_policy (
	account_id   TEXT    NOT NULL,
	strategy_id  TEXT    NOT NULL,
	risk_type    TEXT    NOT NULL,
	lot_size     DOUBLE PRECISION,
	version      BIGINT  NOT NULL DEFAULT 1,
	updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	valid_until  TIMESTAMPTZ,
	PRIMARY KEY (account_id, strategy_id)
);

CREATE INDEX IF NOT EXISTS idx_account_strategy_risk_policy_updated_at
	ON echo.account_strategy_risk_policy (updated_at DESC);

-- Notificar modificaciones para invalidar caché en Core
CREATE OR REPLACE FUNCTION echo.notify_risk_policy_changed() RETURNS TRIGGER AS $$
DECLARE
	acc TEXT;
	strat TEXT;
BEGIN
	acc := COALESCE(NEW.account_id, OLD.account_id, '');
	strat := COALESCE(NEW.strategy_id, OLD.strategy_id, '');
	PERFORM pg_notify('echo_risk_policy_updated', acc || ':' || strat);
	IF TG_OP = 'DELETE' THEN
		RETURN OLD;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_risk_policy_changed ON echo.account_strategy_risk_policy;
CREATE TRIGGER trg_risk_policy_changed
	AFTER INSERT OR UPDATE OR DELETE ON echo.account_strategy_risk_policy
	FOR EACH ROW
	EXECUTE FUNCTION echo.notify_risk_policy_changed();

COMMIT;

