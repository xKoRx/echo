-- ============================================================================
-- Echo i1 - PostgreSQL Schema Teardown
-- ============================================================================
-- Descripción: Limpia completamente el schema de Echo
-- Versión: 1.0.0 (i1)
-- Autor: Echo Team
-- IP Postgres: 192.168.31.220
-- 
-- ⚠️  PELIGRO: Este script ELIMINA TODOS LOS DATOS
-- 
-- Uso:
--   psql -h 192.168.31.220 -U postgres -d echo < teardown.sql
--
-- ============================================================================

-- ============================================================================
-- CONFIRMACIÓN
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '⚠️  WARNING: About to DROP all Echo data';
    RAISE NOTICE '⚠️  Sleeping 3 seconds... Press Ctrl+C to cancel';
    PERFORM pg_sleep(3);
END $$;

-- ============================================================================
-- DROP SCHEMA CASCADE
-- ============================================================================
-- Esto elimina:
-- - Todas las tablas
-- - Todas las funciones
-- - Todas las vistas
-- - Todos los tipos
-- - Todos los índices
-- ============================================================================

DROP SCHEMA IF EXISTS echo CASCADE;

-- ============================================================================
-- CONFIRMACIÓN FINAL
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '✅ Echo schema dropped successfully';
    RAISE NOTICE '📝 Run setup.sql to recreate';
END $$;

