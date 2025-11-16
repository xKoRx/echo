-- Iteración 8ab: soporte para offsets configurables en SL/TP
-- +migrate Up
BEGIN;

ALTER TABLE echo.account_strategy_risk_policy
    ADD COLUMN IF NOT EXISTS sl_offset_pips INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tp_offset_pips INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN echo.account_strategy_risk_policy.sl_offset_pips IS 'Offset (en pips) aplicado al StopLoss del slave (puede ser negativo)';
COMMENT ON COLUMN echo.account_strategy_risk_policy.tp_offset_pips IS 'Offset (en pips) aplicado al TakeProfit del slave (puede ser negativo)';

COMMIT;

-- +migrate Down
BEGIN;

ALTER TABLE echo.account_strategy_risk_policy
    DROP COLUMN IF EXISTS sl_offset_pips,
    DROP COLUMN IF EXISTS tp_offset_pips;

COMMIT;

