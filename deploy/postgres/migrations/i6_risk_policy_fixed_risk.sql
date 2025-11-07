-- Iteración 6: soporte para políticas FIXED_RISK
-- Expand phase: agrega columnas y restricciones necesarias sin remover compatibilidad previa.

-- +migrate Up
BEGIN;

ALTER TABLE echo.account_strategy_risk_policy
    ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE echo.account_strategy_risk_policy
    ADD COLUMN IF NOT EXISTS risk_currency TEXT,
    ADD COLUMN IF NOT EXISTS risk_amount DOUBLE PRECISION;

ALTER TABLE echo.account_strategy_risk_policy
    ADD CONSTRAINT chk_fixed_risk_config
        CHECK (
            risk_type <> 'FIXED_RISK'
            OR (
                (config ? 'amount')
                AND (config ? 'currency')
            )
        );

COMMENT ON COLUMN echo.account_strategy_risk_policy.config IS 'Payload JSONB tipado para políticas de riesgo (Iteración 6)';

-- Trigger existente mantiene notificaciones; no requiere cambios adicionales.

COMMIT;

-- +migrate Down
BEGIN;

ALTER TABLE echo.account_strategy_risk_policy
    DROP CONSTRAINT IF EXISTS chk_fixed_risk_config;

ALTER TABLE echo.account_strategy_risk_policy
    DROP COLUMN IF EXISTS config,
    DROP COLUMN IF EXISTS risk_currency,
    DROP COLUMN IF EXISTS risk_amount;

COMMIT;

