# ADR-004: PostgreSQL para Estado del Sistema

## Estado
**Aprobado** - 2025-10-23

## Contexto

El **Core** necesita mantener estado persistente de:
- Posiciones abiertas por cuenta
- Deduplicación de `trade_id`
- Catálogos (cuentas, símbolos, estrategias)
- Políticas (si no se usa etcd exclusivamente)
- Ventanas de no-ejecución

Opciones consideradas:

1. **PostgreSQL 16**
2. **In-memory** + WAL local
3. **SQLite**
4. **MongoDB**

## Decisión

Usaremos **PostgreSQL 16** desde el inicio.

### Esquema Mínimo

```sql
-- Cuentas
accounts (account_id PK, account_type, broker, is_active)

-- Estrategias
strategies (strategy_id PK, magic_number, name)

-- Símbolos
symbols (canonical_symbol PK)
broker_symbols (broker_symbol PK, canonical_symbol FK, specs...)

-- Estado de posiciones (reconciliación)
positions_state (account_id, ticket, trade_id, symbol, volume, ...)

-- Deduplicación
orders_log (trade_id PK, status, attempts, ...)

-- Ventanas
news_windows (id PK, account_id, symbol, start_utc, end_utc, ...)
```

## Consecuencias

### Positivas
- ✅ **ACID**: Transacciones confiables
- ✅ **Query power**: SQL completo para análisis
- ✅ **Escalabilidad**: Soporta crecimiento futuro
- ✅ **Operacional**: Herramientas maduras (pgAdmin, backups)
- ✅ **JSON support**: JSONB para datos semi-estructurados (políticas)
- ✅ **HA**: Replicación nativa para V2+

### Negativas
- ⚠️ **Overhead**: Más pesado que in-memory o SQLite
- ⚠️ **Dependencia externa**: Requiere servicio separado

### Alternativas Descartadas

**In-memory + WAL**:
- ❌ No ACID robusto
- ❌ Reconstrucción compleja tras crash
- ❌ Sin queries SQL avanzadas

**SQLite**:
- ❌ No concurrencia de escritura
- ❌ No replicación nativa
- ❌ No escalable a multi-instancia (V2+)

**MongoDB**:
- ❌ Overkill para estado estructurado
- ❌ Esquema flexible innecesario aquí
- ✅ **Sí para eventos** (append-only, V2+)

## Implementación

### Conexión

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
)

connStr := "postgres://user:pass@localhost:5432/echo?sslmode=disable"
db, err := sql.Open("postgres", connStr)
defer db.Close()
```

### Pools

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### Migraciones

Usar `golang-migrate` o similar:

```sql
-- 001_initial_schema.up.sql
CREATE TABLE accounts (...);
CREATE INDEX idx_accounts_type ON accounts(account_type);
```

### Queries Preparados

```go
stmt, err := db.PrepareContext(ctx, `
    INSERT INTO positions_state (account_id, ticket, trade_id, ...)
    VALUES ($1, $2, $3, ...)
    ON CONFLICT (account_id, ticket) DO UPDATE ...
`)
defer stmt.Close()
```

## Backup

- **Frecuencia**: Diaria (pg_dump)
- **Retención**: 7 días
- **Point-in-time recovery**: Opcional V2+

## Observabilidad

Métricas:
- `postgres.connections.active`
- `postgres.query.duration`
- `postgres.transactions.committed`

## MongoDB (Futuro V2+)

Para **eventos crudos** (append-only):

```json
{
  "_id": "uuid",
  "event_type": "trade_copied",
  "timestamp": "2025-10-23T10:00:00Z",
  "trade_id": "uuid-v7",
  "master_account": "MT4-001",
  "slaves": ["MT5-001", "MT5-002"],
  "payload": {...}
}
```

## Referencias
- [PostgreSQL 16](https://www.postgresql.org/docs/16/)
- [RFC-001](../RFC-001-architecture.md#43-schema-postgresql-m%C3%ADnimo-v1)

