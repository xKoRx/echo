-- ============================================================================
-- Echo i3 - Migración de Catálogo Canónico de Símbolos
-- ============================================================================
-- Descripción: Tabla para mapeos de símbolos canónicos a símbolos de broker por cuenta
-- Versión: 1.0.0 (i3)
-- Autor: Echo Team
-- RFC: RFC-004-iteracion-3-catalogo-simbolos.md
-- 
-- Uso:
--   psql -h <host> -U postgres -d echo < migrations/i3_symbols.sql
-- ============================================================================

-- ============================================================================
-- TABLA: account_symbol_map
-- ============================================================================
-- Almacena mapeos de símbolos canónicos a símbolos de broker por cuenta.
-- Cada cuenta puede tener múltiples mapeos (uno por símbolo canónico).
-- Los mapeos se actualizan de forma idempotente usando reported_at_ms.
CREATE TABLE IF NOT EXISTS echo.account_symbol_map (
  account_id       TEXT NOT NULL,
  canonical_symbol TEXT NOT NULL,
  broker_symbol    TEXT NOT NULL,
  digits           INTEGER,
  point            DOUBLE PRECISION,
  tick_size        DOUBLE PRECISION,
  min_lot          DOUBLE PRECISION,
  max_lot          DOUBLE PRECISION,
  lot_step         DOUBLE PRECISION,
  stop_level       INTEGER,
  contract_size    DOUBLE PRECISION,  -- Opcional (NULL si no se reporta)
  reported_at_ms   BIGINT NOT NULL DEFAULT 0,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (account_id, canonical_symbol)
);

-- Índices para búsquedas eficientes
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_account ON echo.account_symbol_map(account_id);
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_canonical ON echo.account_symbol_map(canonical_symbol);
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_broker ON echo.account_symbol_map(account_id, broker_symbol);

-- Comentarios de documentación
COMMENT ON TABLE echo.account_symbol_map IS 'Mapeos de símbolos canónicos a símbolos de broker por cuenta (i3)';
COMMENT ON COLUMN echo.account_symbol_map.account_id IS 'Account ID del EA (master o slave)';
COMMENT ON COLUMN echo.account_symbol_map.canonical_symbol IS 'Símbolo canónico normalizado (ej: XAUUSD)';
COMMENT ON COLUMN echo.account_symbol_map.broker_symbol IS 'Símbolo del broker (ej: XAUUSD.m)';
COMMENT ON COLUMN echo.account_symbol_map.reported_at_ms IS 'Timestamp de reporte para idempotencia temporal';
COMMENT ON COLUMN echo.account_symbol_map.contract_size IS 'Tamaño del contrato (opcional, puede ser NULL)';

