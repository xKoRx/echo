-- ======================================================================
-- Script: Normalize UUIDs to Lowercase (i1 Hotfix)
-- Purpose: Fix FK violations caused by case-sensitive UUID comparison
-- Date: 2025-10-29
-- ======================================================================

-- Set search path
SET search_path TO echo, public;

-- ======================================================================
-- 1. Backup counts BEFORE normalization
-- ======================================================================

SELECT 'BEFORE NORMALIZATION' as checkpoint;

SELECT 
    'echo.trades' as table_name,
    COUNT(*) as total_rows,
    COUNT(*) FILTER (WHERE trade_id != LOWER(trade_id)) as uppercase_rows,
    COUNT(*) FILTER (WHERE trade_id = LOWER(trade_id)) as lowercase_rows
FROM echo.trades
UNION ALL
SELECT 
    'echo.executions' as table_name,
    COUNT(*) as total_rows,
    COUNT(*) FILTER (WHERE trade_id != LOWER(trade_id)) as uppercase_rows,
    COUNT(*) FILTER (WHERE trade_id = LOWER(trade_id)) as lowercase_rows
FROM echo.executions
UNION ALL
SELECT 
    'echo.closes' as table_name,
    COUNT(*) as total_rows,
    COUNT(*) FILTER (WHERE trade_id != LOWER(trade_id)) as uppercase_rows,
    COUNT(*) FILTER (WHERE trade_id = LOWER(trade_id)) as lowercase_rows
FROM echo.closes;

-- ======================================================================
-- 2. Show sample of UUIDs that will be affected
-- ======================================================================

SELECT 'Sample UUIDs to be normalized:' as info;

SELECT 
    'trades' as source_table,
    trade_id as current_uuid,
    LOWER(trade_id) as normalized_uuid
FROM echo.trades
WHERE trade_id != LOWER(trade_id)
LIMIT 5;

-- ======================================================================
-- 3. Normalize UUIDs (UPDATE statements)
-- ======================================================================

BEGIN;

-- 3.1. Normalize echo.trades
UPDATE echo.trades
SET trade_id = LOWER(trade_id)
WHERE trade_id != LOWER(trade_id);

-- 3.2. Normalize echo.executions
UPDATE echo.executions
SET trade_id = LOWER(trade_id)
WHERE trade_id != LOWER(trade_id);

-- 3.3. Normalize echo.closes (if any exist)
UPDATE echo.closes
SET trade_id = LOWER(trade_id)
WHERE trade_id != LOWER(trade_id);

-- 3.4. Normalize echo.dedupe (if it has trade_id)
-- UPDATE echo.dedupe
-- SET trade_id = LOWER(trade_id)
-- WHERE trade_id != LOWER(trade_id);

COMMIT;

-- ======================================================================
-- 4. Verification AFTER normalization
-- ======================================================================

SELECT 'AFTER NORMALIZATION' as checkpoint;

SELECT 
    'echo.trades' as table_name,
    COUNT(*) as total_rows,
    COUNT(*) FILTER (WHERE trade_id != LOWER(trade_id)) as uppercase_rows,
    COUNT(*) FILTER (WHERE trade_id = LOWER(trade_id)) as lowercase_rows
FROM echo.trades
UNION ALL
SELECT 
    'echo.executions' as table_name,
    COUNT(*) as total_rows,
    COUNT(*) FILTER (WHERE trade_id != LOWER(trade_id)) as uppercase_rows,
    COUNT(*) FILTER (WHERE trade_id = LOWER(trade_id)) as lowercase_rows
FROM echo.executions
UNION ALL
SELECT 
    'echo.closes' as table_name,
    COUNT(*) as total_rows,
    COUNT(*) FILTER (WHERE trade_id != LOWER(trade_id)) as uppercase_rows,
    COUNT(*) FILTER (WHERE trade_id = LOWER(trade_id)) as lowercase_rows
FROM echo.closes;

-- ======================================================================
-- 5. Validate FK integrity
-- ======================================================================

SELECT 'FK VALIDATION' as checkpoint;

-- 5.1. Check all executions have matching trades
SELECT 
    'executions_without_trades' as issue,
    COUNT(*) as orphan_count
FROM echo.executions e
LEFT JOIN echo.trades t ON e.trade_id = t.trade_id
WHERE t.trade_id IS NULL;

-- 5.2. Check all closes have matching trades
SELECT 
    'closes_without_trades' as issue,
    COUNT(*) as orphan_count
FROM echo.closes c
LEFT JOIN echo.trades t ON c.trade_id = t.trade_id
WHERE t.trade_id IS NULL;

-- ======================================================================
-- SUCCESS MESSAGE
-- ======================================================================

SELECT 
    CASE 
        WHEN 
            (SELECT COUNT(*) FROM echo.trades WHERE trade_id != LOWER(trade_id)) = 0
            AND
            (SELECT COUNT(*) FROM echo.executions WHERE trade_id != LOWER(trade_id)) = 0
            AND
            (SELECT COUNT(*) FROM echo.closes WHERE trade_id != LOWER(trade_id)) = 0
        THEN '✅ All UUIDs normalized successfully'
        ELSE '⚠️ Some UUIDs still in uppercase, check logs'
    END as status;

-- ======================================================================
-- ROLLBACK (in case of emergency)
-- ======================================================================

-- If you need to rollback (before COMMIT):
-- ROLLBACK;

