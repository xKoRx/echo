-- ============================================================================
-- Echo i1 - PostgreSQL Schema Setup
-- ============================================================================
-- Descripción: Schema para persistencia de operaciones, dedupe y correlación
-- Versión: 1.0.0 (i1)
-- Autor: Echo Team
-- IP Postgres: 192.168.31.220
-- 
-- Uso:
--   psql -h 192.168.31.220 -U postgres -d echo < setup.sql
--
-- Variables de control:
--   DROP_IF_EXISTS: si es true, hace DROP SCHEMA CASCADE antes de crear
-- ============================================================================

-- ============================================================================
-- CONTROL: Comentar/descomentar según necesidad
-- ============================================================================
-- DROP SCHEMA IF EXISTS echo CASCADE;  -- ⚠️ Descomentar solo para recrear desde cero

-- ============================================================================
-- SCHEMA
-- ============================================================================
CREATE SCHEMA IF NOT EXISTS echo;

-- ============================================================================
-- EXTENSIONES
-- ============================================================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";      -- Para validaciones UUID
CREATE EXTENSION IF NOT EXISTS "pg_trgm";        -- Para búsquedas text similarity

-- ============================================================================
-- TIPOS ENUM
-- ============================================================================

-- Estado de una orden en el sistema
CREATE TYPE echo.order_status AS ENUM (
    'PENDING',      -- Intención recibida, no enviada aún
    'SENT',         -- Comando enviado al slave
    'FILLED',       -- Confirmación de fill recibida
    'REJECTED',     -- Rechazada por slave/broker
    'CANCELLED'     -- Cancelada manualmente
);

-- Lado de la orden
CREATE TYPE echo.order_side AS ENUM (
    'BUY',
    'SELL'
);

-- ============================================================================
-- TABLA: trades
-- ============================================================================
-- Almacena las intenciones de trade desde el Master
-- Una fila = un trade_id único del Master
-- ============================================================================
CREATE TABLE IF NOT EXISTS echo.trades (
    -- Identidad
    trade_id            TEXT PRIMARY KEY,           -- UUIDv7 del trade
    source_master_id    TEXT NOT NULL,              -- ID del Master EA
    master_account_id   TEXT NOT NULL,              -- Account ID del master
    master_ticket       INTEGER NOT NULL,           -- Ticket del master
    
    -- Detalles del trade
    magic_number        BIGINT NOT NULL,            -- MagicNumber MT4/MT5
    symbol              TEXT NOT NULL,              -- Símbolo (ej: XAUUSD)
    side                echo.order_side NOT NULL,   -- BUY/SELL
    lot_size            DOUBLE PRECISION NOT NULL,  -- Tamaño en lotes del master
    price               DOUBLE PRECISION NOT NULL,  -- Precio de apertura en master
    
    -- SL/TP opcionales
    stop_loss           DOUBLE PRECISION,
    take_profit         DOUBLE PRECISION,
    comment             TEXT,
    
    -- Estado
    status              echo.order_status NOT NULL DEFAULT 'PENDING',
    attempt             INTEGER NOT NULL DEFAULT 0,  -- Número de intento
    
    -- Timestamps
    opened_at_ms        BIGINT NOT NULL,             -- Timestamp de apertura en master
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Índices para trades
CREATE INDEX IF NOT EXISTS idx_trades_master_account 
    ON echo.trades(master_account_id);

CREATE INDEX IF NOT EXISTS idx_trades_master_ticket 
    ON echo.trades(master_ticket);

CREATE INDEX IF NOT EXISTS idx_trades_magic_number 
    ON echo.trades(magic_number);

CREATE INDEX IF NOT EXISTS idx_trades_symbol 
    ON echo.trades(symbol);

CREATE INDEX IF NOT EXISTS idx_trades_status 
    ON echo.trades(status);

CREATE INDEX IF NOT EXISTS idx_trades_created_at 
    ON echo.trades(created_at DESC);

-- ============================================================================
-- TABLA: executions
-- ============================================================================
-- Almacena las ejecuciones en slaves
-- Una fila = un resultado de ExecuteOrder en un slave específico
-- ============================================================================
CREATE TABLE IF NOT EXISTS echo.executions (
    -- Identidad
    execution_id        TEXT PRIMARY KEY,               -- UUID del execution (command_id)
    trade_id            TEXT NOT NULL,                  -- FK a trades
    
    -- Slave info
    slave_account_id    TEXT NOT NULL,                  -- Account ID del slave
    agent_id            TEXT NOT NULL,                  -- ID del agent que ejecutó
    
    -- Resultado de ejecución
    slave_ticket        INTEGER NOT NULL,               -- Ticket generado en slave (0 si fallo)
    executed_price      DOUBLE PRECISION,               -- Precio ejecutado (NULL si fallo)
    success             BOOLEAN NOT NULL,               -- true = fill, false = reject
    error_code          TEXT NOT NULL DEFAULT 'NONE',  -- Código de error (ej: ERR_NO_ERROR, ERR_REQUOTE)
    error_message       TEXT NOT NULL DEFAULT '',      -- Mensaje de error
    
    -- Latencia E2E (timestamps t0..t7)
    timestamps_ms       JSONB NOT NULL,                 -- {t0: 123, t1: 124, ...}
    
    -- Timestamps
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- FK
    CONSTRAINT fk_executions_trade 
        FOREIGN KEY (trade_id) 
        REFERENCES echo.trades(trade_id) 
        ON DELETE CASCADE
);

-- Índice único para correlación determinística: trade_id + slave_account + ticket
-- Garantiza que podemos identificar de forma única cada posición abierta
CREATE UNIQUE INDEX IF NOT EXISTS ux_execution_trade_slave_ticket
    ON echo.executions (trade_id, slave_account_id, slave_ticket)
    WHERE slave_ticket != 0;  -- Solo para ejecuciones exitosas

-- Índices para executions
CREATE INDEX IF NOT EXISTS idx_executions_trade_id 
    ON echo.executions(trade_id);

CREATE INDEX IF NOT EXISTS idx_executions_slave_account 
    ON echo.executions(slave_account_id);

CREATE INDEX IF NOT EXISTS idx_executions_success 
    ON echo.executions(success);

CREATE INDEX IF NOT EXISTS idx_executions_created_at 
    ON echo.executions(created_at DESC);

-- ============================================================================
-- TABLA: dedupe
-- ============================================================================
-- Almacena estado de deduplicación por trade_id
-- TTL: 1 hora para estados terminales (FILLED, REJECTED, CANCELLED)
-- ============================================================================
CREATE TABLE IF NOT EXISTS echo.dedupe (
    -- Identidad
    trade_id            TEXT PRIMARY KEY,               -- UUIDv7 del trade
    
    -- Estado
    status              echo.order_status NOT NULL,     -- Estado actual del trade
    
    -- Timestamps
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Índice para cleanup por TTL
CREATE INDEX IF NOT EXISTS idx_dedupe_updated_at 
    ON echo.dedupe(updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_dedupe_status 
    ON echo.dedupe(status);

-- ============================================================================
-- TABLA: closes
-- ============================================================================
-- Almacena auditoría de cierres de posiciones
-- Una fila = un cierre de posición en un slave
-- ============================================================================
CREATE TABLE IF NOT EXISTS echo.closes (
    -- Identidad
    close_id            TEXT PRIMARY KEY,               -- UUID del close command
    trade_id            TEXT NOT NULL,                  -- FK a trades
    
    -- Slave info
    slave_account_id    TEXT NOT NULL,                  -- Account ID del slave
    slave_ticket        INTEGER NOT NULL,               -- Ticket cerrado en slave
    
    -- Resultado de cierre
    close_price         DOUBLE PRECISION,               -- Precio de cierre (NULL si fallo)
    success             BOOLEAN NOT NULL,               -- true = cerrado, false = error
    error_code          TEXT NOT NULL DEFAULT 'NONE',  -- Código de error
    error_message       TEXT NOT NULL DEFAULT '',      -- Mensaje de error
    
    -- Timestamps
    closed_at_ms        BIGINT NOT NULL,                -- Timestamp de cierre
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- FK
    CONSTRAINT fk_closes_trade 
        FOREIGN KEY (trade_id) 
        REFERENCES echo.trades(trade_id) 
        ON DELETE CASCADE
);

-- Índices para closes
CREATE INDEX IF NOT EXISTS idx_closes_trade_id 
    ON echo.closes(trade_id);

CREATE INDEX IF NOT EXISTS idx_closes_slave_account 
    ON echo.closes(slave_account_id);

CREATE INDEX IF NOT EXISTS idx_closes_slave_ticket 
    ON echo.closes(slave_ticket);

CREATE INDEX IF NOT EXISTS idx_closes_created_at 
    ON echo.closes(created_at DESC);

-- ============================================================================
-- TABLA: config (para ETCD fallback o cache local)
-- ============================================================================
-- Opcional: cache de configuración ETCD para debugging
-- No se usa en hot path, solo para inspección
-- ============================================================================
CREATE TABLE IF NOT EXISTS echo.config_cache (
    key                 TEXT PRIMARY KEY,
    value               TEXT NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- FUNCIONES AUXILIARES
-- ============================================================================

-- Función para actualizar updated_at automáticamente
CREATE OR REPLACE FUNCTION echo.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers para updated_at
CREATE TRIGGER update_trades_updated_at 
    BEFORE UPDATE ON echo.trades
    FOR EACH ROW EXECUTE FUNCTION echo.update_updated_at_column();

CREATE TRIGGER update_dedupe_updated_at 
    BEFORE UPDATE ON echo.dedupe
    FOR EACH ROW EXECUTE FUNCTION echo.update_updated_at_column();

-- ============================================================================
-- FUNCIÓN: Cleanup dedupe entries antiguos (TTL)
-- ============================================================================
-- Limpia entries de dedupe con estados terminales más antiguos de 1 hora
-- Uso: SELECT echo.cleanup_dedupe_ttl();
-- ============================================================================
CREATE OR REPLACE FUNCTION echo.cleanup_dedupe_ttl()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM echo.dedupe
    WHERE status IN ('FILLED', 'REJECTED', 'CANCELLED')
      AND updated_at < NOW() - INTERVAL '1 hour';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- VISTAS ÚTILES
-- ============================================================================

-- Vista: resumen de trades con conteo de executions
CREATE OR REPLACE VIEW echo.v_trades_summary AS
SELECT 
    t.trade_id,
    t.master_account_id,
    t.master_ticket,
    t.symbol,
    t.side,
    t.status,
    t.created_at,
    COUNT(e.execution_id) as total_executions,
    SUM(CASE WHEN e.success THEN 1 ELSE 0 END) as successful_executions,
    SUM(CASE WHEN NOT e.success THEN 1 ELSE 0 END) as failed_executions
FROM echo.trades t
LEFT JOIN echo.executions e ON t.trade_id = e.trade_id
GROUP BY t.trade_id, t.master_account_id, t.master_ticket, 
         t.symbol, t.side, t.status, t.created_at
ORDER BY t.created_at DESC;

-- Vista: latencias promedio por hop
CREATE OR REPLACE VIEW echo.v_latency_stats AS
SELECT 
    COUNT(*) as total_executions,
    AVG((timestamps_ms->>'t1')::bigint - (timestamps_ms->>'t0')::bigint) as avg_master_to_agent_ms,
    AVG((timestamps_ms->>'t2')::bigint - (timestamps_ms->>'t1')::bigint) as avg_agent_to_core_ms,
    AVG((timestamps_ms->>'t3')::bigint - (timestamps_ms->>'t2')::bigint) as avg_core_process_ms,
    AVG((timestamps_ms->>'t4')::bigint - (timestamps_ms->>'t3')::bigint) as avg_core_to_agent_ms,
    AVG((timestamps_ms->>'t5')::bigint - (timestamps_ms->>'t4')::bigint) as avg_agent_to_slave_ms,
    AVG((timestamps_ms->>'t6')::bigint - (timestamps_ms->>'t5')::bigint) as avg_slave_process_ms,
    AVG((timestamps_ms->>'t7')::bigint - (timestamps_ms->>'t6')::bigint) as avg_order_fill_ms,
    AVG((timestamps_ms->>'t7')::bigint - (timestamps_ms->>'t0')::bigint) as avg_e2e_ms
FROM echo.executions
WHERE success = true
  AND timestamps_ms IS NOT NULL;

-- ============================================================================
-- PERMISOS (ajustar según usuario de la app)
-- ============================================================================
-- GRANT USAGE ON SCHEMA echo TO echo_user;
-- GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA echo TO echo_user;
-- GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA echo TO echo_user;
-- GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA echo TO echo_user;

-- ============================================================================
-- DATOS DE EJEMPLO (SOLO PARA DESARROLLO)
-- ============================================================================
-- Descomentar para poblar con datos de prueba

/*
INSERT INTO echo.trades (trade_id, source_master_id, master_account_id, master_ticket, 
                         magic_number, symbol, side, lot_size, price, opened_at_ms)
VALUES 
    ('01HKQ000-0000-7000-8000-000000000001', 'master-ea-1', '12345', 1001, 
     123456, 'XAUUSD', 'BUY', 0.10, 2050.50, 1730000000000),
    ('01HKQ000-0000-7000-8000-000000000002', 'master-ea-1', '12345', 1002, 
     123456, 'XAUUSD', 'SELL', 0.10, 2051.00, 1730000060000);

INSERT INTO echo.dedupe (trade_id, status)
VALUES 
    ('01HKQ000-0000-7000-8000-000000000001', 'FILLED'),
    ('01HKQ000-0000-7000-8000-000000000002', 'FILLED');
*/

-- ============================================================================
-- FIN DEL SCRIPT
-- ============================================================================

-- Verificar creación
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables
WHERE schemaname = 'echo'
ORDER BY tablename;

-- Mensajes de confirmación
DO $$
BEGIN
    RAISE NOTICE '✅ Echo i1 schema created successfully';
    RAISE NOTICE '📊 Tables: trades, executions, dedupe, closes, config_cache';
    RAISE NOTICE '🔍 Views: v_trades_summary, v_latency_stats';
    RAISE NOTICE '🧹 Function: cleanup_dedupe_ttl()';
END $$;

