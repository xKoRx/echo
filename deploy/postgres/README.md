# Echo PostgreSQL Deployment

## 📋 Descripción

Scripts de setup y teardown para la base de datos PostgreSQL de Echo i1.

## 🎯 Requisitos

- PostgreSQL 16+
- Usuario con permisos de CREATE SCHEMA
- IP del servidor: `192.168.31.220`

## 🚀 Setup Inicial

### Opción 1: Setup limpio (primera vez)

```bash
# Conectar y ejecutar setup
psql -h 192.168.31.220 -U postgres -d echo -f setup.sql
```

### Opción 2: Recrear desde cero (desarrollo)

```bash
# 1. Eliminar schema existente
psql -h 192.168.31.220 -U postgres -d echo -f teardown.sql

# 2. Crear schema nuevo
psql -h 192.168.31.220 -U postgres -d echo -f setup.sql
```

### Opción 3: Un solo comando (desarrollo rápido)

```bash
# Teardown + Setup en pipeline
psql -h 192.168.31.220 -U postgres -d echo -f teardown.sql && \
psql -h 192.168.31.220 -U postgres -d echo -f setup.sql
```

## 📊 Schema Overview

### Tablas Principales

| Tabla | Descripción | Uso |
|-------|-------------|-----|
| `echo.trades` | Intenciones de trade del Master | Almacena cada trade_id único con metadatos |
| `echo.executions` | Ejecuciones en slaves | Una fila por cada ExecuteOrder enviado a un slave |
| `echo.dedupe` | Estado de deduplicación | Evita duplicados por trade_id con estados persistentes |
| `echo.closes` | Auditoría de cierres | Registro de cada CloseOrder ejecutado |
| `echo.config_cache` | Cache de config ETCD | Opcional, para debugging |

### Índices Clave

- **Correlación trade → tickets**: `ux_execution_trade_slave_ticket` (único)
- **Búsqueda por account**: `idx_trades_master_account`, `idx_executions_slave_account`
- **Búsqueda por ticket**: `idx_trades_master_ticket`, `idx_closes_slave_ticket`
- **Cleanup dedupe**: `idx_dedupe_updated_at`, `idx_dedupe_status`

### Vistas

- **`echo.v_trades_summary`**: Resumen de trades con conteo de executions
- **`echo.v_latency_stats`**: Estadísticas de latencia por hop (t0..t7)

### Funciones

- **`echo.cleanup_dedupe_ttl()`**: Limpia entries de dedupe con TTL > 1 hora

```sql
-- Ejecutar cleanup manual
SELECT echo.cleanup_dedupe_ttl();
```

## 🔍 Queries Útiles

### Ver trades recientes

```sql
SELECT * FROM echo.v_trades_summary 
ORDER BY created_at DESC 
LIMIT 10;
```

### Ver latencias promedio

```sql
SELECT * FROM echo.v_latency_stats;
```

### Buscar executions de un trade

```sql
SELECT 
    e.slave_account_id,
    e.slave_ticket,
    e.success,
    e.error_code,
    e.timestamps_ms
FROM echo.executions e
WHERE e.trade_id = '01HKQ...';
```

### Correlación: dado un trade_id del master, encontrar tickets de slaves

```sql
SELECT 
    trade_id,
    slave_account_id,
    slave_ticket,
    executed_price,
    success
FROM echo.executions
WHERE trade_id = '01HKQ...'
  AND success = true
ORDER BY created_at;
```

### Dedupe: verificar si un trade_id ya fue procesado

```sql
SELECT trade_id, status, updated_at
FROM echo.dedupe
WHERE trade_id = '01HKQ...';
```

### Closes: auditoría de cierres por trade

```sql
SELECT 
    c.close_id,
    c.slave_account_id,
    c.slave_ticket,
    c.close_price,
    c.success,
    c.closed_at_ms
FROM echo.closes c
WHERE c.trade_id = '01HKQ...'
ORDER BY c.created_at;
```

## 🧹 Mantenimiento

### Cleanup dedupe periódico

El Core ejecuta automáticamente cleanup cada 1 minuto vía código Go.

Manualmente:

```sql
SELECT echo.cleanup_dedupe_ttl();
```

### Vacuum y analyze

```sql
VACUUM ANALYZE echo.trades;
VACUUM ANALYZE echo.executions;
VACUUM ANALYZE echo.dedupe;
VACUUM ANALYZE echo.closes;
```

## 🔐 Permisos

Si usas un usuario específico (`echo_user`), descomentar en `setup.sql`:

```sql
GRANT USAGE ON SCHEMA echo TO echo_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA echo TO echo_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA echo TO echo_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA echo TO echo_user;
```

## ⚠️  Advertencias

- **Teardown**: `DROP SCHEMA CASCADE` elimina **TODOS** los datos irreversiblemente
- **Backup**: Hacer backup antes de teardown en producción
- **TTL dedupe**: Por defecto 1 hora para estados terminales (ajustable en código)

## 🎯 Uso en Desarrollo

Para iterar rápido durante i1:

```bash
# Loop de desarrollo
alias echo-db-reset="psql -h 192.168.31.220 -U postgres -d echo -f teardown.sql && psql -h 192.168.31.220 -U postgres -d echo -f setup.sql"

# Ejecutar
echo-db-reset
```

## 📝 Notas de Iteración

- **i0**: Sin persistencia (in-memory)
- **i1**: Primera iteración con PostgreSQL para dedupe + correlación
- **i2+**: Normalización adicional si es necesario

## 🔗 Referencias

- RFC-003: Iteración 1 Implementation
- RFC-001: Architecture Overview

