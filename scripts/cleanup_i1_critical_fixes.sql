-- ============================================================================
-- Script de Limpieza: Critical Fixes i1
-- ============================================================================
-- Propósito: Limpiar datos incorrectos generados por bugs de i1:
--   1. Closes con trade_id inexistente (FK violations)
--   2. Dedupe entries con status CANCELLED incorrecto
--   3. Dedupe entries huérfanas (trade_id generado por bug, sin execution)
--
-- Fecha: 2025-10-29
-- Versión: 1.0
-- Ejecutar en: PostgreSQL 16
-- Schema: echo
--
-- IMPORTANTE: Ejecutar DESPUÉS de desplegar los fixes en Master EA y Core
-- ============================================================================

-- Mostrar estadísticas ANTES de la limpieza
SELECT '=== BEFORE CLEANUP ===' AS step;

SELECT 'Total closes' AS metric, COUNT(*) AS count FROM echo.closes;
SELECT 'Closes con FK violation (trade_id inexistente)' AS metric, COUNT(*) AS count
FROM echo.closes c
WHERE NOT EXISTS (SELECT 1 FROM echo.trades t WHERE t.trade_id = c.trade_id);

SELECT 'Total dedupe' AS metric, COUNT(*) AS count FROM echo.dedupe;
SELECT 'Dedupe PENDING' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'PENDING';
SELECT 'Dedupe SENT' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'SENT';
SELECT 'Dedupe FILLED' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'FILLED';
SELECT 'Dedupe REJECTED' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'REJECTED';
SELECT 'Dedupe CANCELLED (INCORRECTO)' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'CANCELLED';

SELECT 'Dedupe huérfanas (sin execution ni trade)' AS metric, COUNT(*) AS count
FROM echo.dedupe d
WHERE NOT EXISTS (SELECT 1 FROM echo.executions e WHERE e.trade_id = d.trade_id)
  AND NOT EXISTS (SELECT 1 FROM echo.trades t WHERE t.trade_id = d.trade_id);

-- ============================================================================
-- LIMPIEZA
-- ============================================================================

BEGIN;

-- ----------------------------------------------------------------------------
-- 1. ELIMINAR closes con trade_id inexistente
-- ----------------------------------------------------------------------------
-- Estas son closes que intentaron insertarse con un trade_id generado por el
-- Master EA al cerrar (bug: generaba nuevo UUID en lugar de reutilizar).
-- Fallarían con FK constraint si no se hubiera eliminado el FK temporalmente.
-- ----------------------------------------------------------------------------

WITH deleted_closes AS (
    DELETE FROM echo.closes
    WHERE trade_id NOT IN (SELECT trade_id FROM echo.trades)
    RETURNING close_id, trade_id, slave_account_id, slave_ticket
)
SELECT 
    '1. Closes eliminadas (trade_id inexistente)' AS step,
    COUNT(*) AS count,
    array_agg(trade_id ORDER BY trade_id) FILTER (WHERE trade_id IS NOT NULL) AS sample_trade_ids
FROM deleted_closes;


-- ----------------------------------------------------------------------------
-- 2. CORREGIR dedupe: CANCELLED → FILLED
-- ----------------------------------------------------------------------------
-- Dedupe entries marcadas como CANCELLED pero que en realidad son órdenes
-- ejecutadas exitosamente (FILLED) que luego fueron cerradas.
-- El bug marcaba incorrectamente como CANCELLED al registrar el cierre.
-- ----------------------------------------------------------------------------

WITH updated_dedupe AS (
    UPDATE echo.dedupe d
    SET status = 'FILLED'
    WHERE d.status = 'CANCELLED'
      AND EXISTS (
        SELECT 1 FROM echo.executions e
        WHERE e.trade_id = d.trade_id
          AND e.success = true
      )
    RETURNING trade_id
)
SELECT 
    '2. Dedupe corregidas (CANCELLED → FILLED)' AS step,
    COUNT(*) AS count,
    array_agg(trade_id ORDER BY trade_id) FILTER (WHERE trade_id IS NOT NULL) AS sample_trade_ids
FROM updated_dedupe;


-- ----------------------------------------------------------------------------
-- 3. ELIMINAR dedupe huérfanas
-- ----------------------------------------------------------------------------
-- Entries de dedupe con trade_id que no existen ni en executions ni en trades.
-- Estas son producto del bug: Master generaba UUID nuevo al cerrar, se creaba
-- entry de dedupe con ese UUID nuevo pero nunca se ejecutó realmente.
-- ----------------------------------------------------------------------------

WITH deleted_dedupe AS (
    DELETE FROM echo.dedupe
    WHERE trade_id NOT IN (SELECT trade_id FROM echo.executions)
      AND trade_id NOT IN (SELECT trade_id FROM echo.trades)
    RETURNING trade_id, status
)
SELECT 
    '3. Dedupe huérfanas eliminadas' AS step,
    COUNT(*) AS count,
    COUNT(*) FILTER (WHERE status = 'CANCELLED') AS cancelled_count,
    array_agg(trade_id ORDER BY trade_id) FILTER (WHERE trade_id IS NOT NULL) AS sample_trade_ids
FROM deleted_dedupe;


-- ============================================================================
-- VERIFICACIÓN POST-LIMPIEZA
-- ============================================================================

SELECT '=== AFTER CLEANUP ===' AS step;

-- Verificar: NO deben quedar closes con FK violation
SELECT 'Closes con FK violation (debe ser 0)' AS verification, COUNT(*) AS count
FROM echo.closes c
WHERE NOT EXISTS (SELECT 1 FROM echo.trades t WHERE t.trade_id = c.trade_id);

-- Verificar: Estadísticas de dedupe
SELECT 'Dedupe PENDING' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'PENDING';
SELECT 'Dedupe SENT' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'SENT';
SELECT 'Dedupe FILLED' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'FILLED';
SELECT 'Dedupe REJECTED' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'REJECTED';
SELECT 'Dedupe CANCELLED (debe reducirse drásticamente)' AS metric, COUNT(*) AS count FROM echo.dedupe WHERE status = 'CANCELLED';

-- Verificar: Total de closes restantes
SELECT 'Closes totales (válidas)' AS metric, COUNT(*) AS count FROM echo.closes;

-- Verificar: NO deben quedar dedupe huérfanas
SELECT 'Dedupe huérfanas (debe ser 0)' AS verification, COUNT(*) AS count
FROM echo.dedupe d
WHERE NOT EXISTS (SELECT 1 FROM echo.executions e WHERE e.trade_id = d.trade_id)
  AND NOT EXISTS (SELECT 1 FROM echo.trades t WHERE t.trade_id = d.trade_id);

-- ============================================================================
-- ANÁLISIS ADICIONAL (OPCIONAL)
-- ============================================================================

-- Ver últimas closes válidas
SELECT 'Últimas 5 closes válidas' AS info;
SELECT close_id, trade_id, slave_account_id, slave_ticket, success, closed_at_ms
FROM echo.closes
ORDER BY created_at DESC
LIMIT 5;

-- Ver distribución de estados de dedupe
SELECT 'Distribución de estados de dedupe' AS info;
SELECT status, COUNT(*) AS count, 
       ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 2) AS percentage
FROM echo.dedupe
GROUP BY status
ORDER BY count DESC;

-- Ver correlación trade_id → executions
SELECT 'Trades con executions' AS info;
SELECT 
    t.trade_id,
    t.status AS trade_status,
    d.status AS dedupe_status,
    COUNT(e.execution_id) AS executions_count,
    COUNT(c.close_id) AS closes_count
FROM echo.trades t
LEFT JOIN echo.dedupe d ON d.trade_id = t.trade_id
LEFT JOIN echo.executions e ON e.trade_id = t.trade_id
LEFT JOIN echo.closes c ON c.trade_id = t.trade_id
GROUP BY t.trade_id, t.status, d.status
ORDER BY t.created_at DESC
LIMIT 10;

-- ============================================================================
-- COMMIT O ROLLBACK
-- ============================================================================

-- Si todas las verificaciones son correctas (ver output), hacer COMMIT.
-- Si algo se ve mal, hacer ROLLBACK.

COMMIT;
-- ROLLBACK;  -- Descomentar si se necesita revertir

-- ============================================================================
-- POST-COMMIT: Restaurar FK constraint si fue eliminada
-- ============================================================================

-- Si habías eliminado el FK constraint temporalmente para evitar errores,
-- ahora puedes restaurarlo:

/*
ALTER TABLE echo.closes
ADD CONSTRAINT closes_trade_id_fkey
FOREIGN KEY (trade_id) REFERENCES echo.trades(trade_id) ON DELETE CASCADE;
*/

-- Verificar constraint
SELECT 'FK constraint verificada' AS status;
SELECT conname, contype, confrelid::regclass AS referenced_table
FROM pg_constraint
WHERE conrelid = 'echo.closes'::regclass
  AND conname = 'closes_trade_id_fkey';

-- ============================================================================
-- FIN DEL SCRIPT
-- ============================================================================

SELECT '=== CLEANUP COMPLETADO ===' AS final_status;
SELECT NOW() AS timestamp;

