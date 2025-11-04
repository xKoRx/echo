---
title: "RFC-004: Catálogo canónico de símbolos y mapeo por cuenta (Iteración 3)"
version: "1.0"
date: "2025-10-30"
status: "Aprobado"
owner: "Arquitectura Echo"
iteration: "3"
replaces: "Validación por whitelist plana (core/symbol_whitelist)"
---

## Resumen Ejecutivo

Esta RFC define la Iteración 3: establecer un **catálogo canónico de símbolos** (desde ETCD como configuración, sin uso de ETCD como caché) y un **mapeo por cuenta** (`canonical_symbol ⇄ broker_symbol`) reportado por los Agents/EAs. El Core validará símbolos canónicos y traducirá a símbolos de broker por cuenta antes de enviar comandos a los Slaves. Se separan responsabilidades en servicios específicos, se define concurrencia y warm-up, y se instrumenta observabilidad.

Objetivos medibles (i3):
- 0 errores por símbolo desconocido en flujo normal (cuando `unknown_action=reject`), con transición segura desde `warn`.
- Mapeo persistido y trazable (consultable por cuenta y por símbolo) con índices adecuados.
- Validación y traducción de símbolo previa a `ExecuteOrder`/`CloseOrder` con lookup O(1) en caché en memoria.

Apoyos previos:
- `SymbolInfo` ya existe en el proto.
- `AgentHello` y `AccountConnected/Disconnected` operativos; `AgentHello.symbols` queda reservado/deprecado (no se usa en i3).

Referencias del roadmap:

```157:162:/home/kor/go/src/github.com/xKoRx/echo/docs/01-arquitectura-y-roadmap.md
Iteración 3 — Catálogo canónico de símbolos y mapeo por cuenta
- Objetivo: estandarizar `canonical_symbol ⇄ broker_symbol` por cuenta.
- Alcance: catálogo canónico en Core; agent/EA reportan mapeo del broker al conectar; validaciones previas al envío.
- Exclusiones: sizing y políticas.
- Criterios de salida: 0 errores por símbolo desconocido; mapeo persistido y trazable.
```

## 1. Contexto y Motivación

### 1.1 Problema
- Los brokers usan sufijos/prefijos y convenciones distintas de símbolos, generando fallas de validación y ejecución si no se mapean correctamente.
- El Core valida actualmente por whitelist plana y reenvía el `symbol` recibido del master a los slaves, sin traducir por cuenta.

Código actual relevante:

```214:221:/home/kor/go/src/github.com/xKoRx/echo/core/internal/router.go
// 2. Validar símbolo (usando SDK)
if err := domain.ValidateSymbol(intent.Symbol, r.core.config.SymbolWhitelist); err != nil {
    r.core.telemetry.Warn(ctx, "Invalid symbol, TradeIntent rejected",
        attribute.String("error", err.Error()),
    )
    // TODO i1: enviar rechazo al Agent
    return
}
```

```146:154:/home/kor/go/src/github.com/xKoRx/echo/core/internal/config.go
if val, err := etcdClient.GetVarWithDefault(ctx, "core/symbol_whitelist", ""); err == nil && val != "" {
    cfg.SymbolWhitelist = strings.Split(val, ",")
    // Trim spaces
    for i := range cfg.SymbolWhitelist {
        cfg.SymbolWhitelist[i] = strings.TrimSpace(cfg.SymbolWhitelist[i])
    }
}
```

```540:570:/home/kor/go/src/github.com/xKoRx/echo/sdk/domain/transformers.go
// TradeIntent → ExecuteOrder (símbolo copiado tal cual del intent)
order := &pb.ExecuteOrder{
    CommandId:       opts.CommandID,
    TradeId:         intent.TradeId,
    TimestampMs:     utils.NowUnixMilli(),
    TargetClientId:  opts.ClientID,
    TargetAccountId: opts.AccountID,
    Symbol:          intent.Symbol,
    Side:            intent.Side,
    LotSize:         opts.LotSize,
    MagicNumber:     intent.MagicNumber,
    Timestamps:      cloneTimestamps(intent.Timestamps),
}
```

```282:307:/home/kor/go/src/github.com/xKoRx/echo/agent/internal/stream.go
// NEW i2: sendAgentHello envía el handshake inicial (solo metadata).
func (a *Agent) sendAgentHello() error {
    hostname, _ := os.Hostname() // Excepción permitida por reglas

    hello := &pb.AgentHello{
        AgentId:  a.config.AgentID,
        Version:  a.config.ServiceVersion,
        Hostname: hostname,
        Os:       runtime.GOOS,
        Symbols:  make(map[string]*pb.SymbolInfo), // TODO i3: reportar símbolos
    }
    // ...
}
```

### 1.2 Objetivo de la Iteración 3
Establecer un catálogo canónico gestionado por el Core, y un mapeo por cuenta proveniente del Agent/EA, de manera que el Core valide y traduzca símbolos antes de enviar órdenes a cada slave.

## 2. Alcance y Exclusiones

### 2.1 En Alcance
- Proto (SDK): nuevo mensaje para reporte de símbolos por cuenta; uso de `SymbolInfo` existente.
- Agent: reporte de mapeos por cuenta al conectar (o inmediatamente luego).
- Core: servicio de catálogo de símbolos, persistencia de mapeos por cuenta, caché en memoria, validación y traducción de símbolos en `Router`.
- ETCD: clave de "catálogo canónico" como lista (whitelist canónica) y parámetros de validación.
- Observabilidad: métricas/logs de validación y de resoluciones hit/miss de mapeo.

### 2.2 Fuera de Alcance
- Money Management (i5), tolerancias (i6), SL/TP (i7), ventanas (i8), SL catastrófico (i9).
- Normalización de códigos de error (i11).
- Cambios en pricing/tick value (se tratan en i4/i5).

## 3. Arquitectura de Solución

### 3.1 Contratos (SDK)

- Se mantiene `SymbolInfo` (proto) como especificación por símbolo del broker y su canónico.

```141:155:/home/kor/go/src/github.com/xKoRx/echo/sdk/proto/v1/trade.proto
// SymbolInfo especificaciones de un símbolo del broker
message SymbolInfo {
  string broker_symbol = 1;      // Nombre del símbolo en el broker
  string canonical_symbol = 2;   // Nombre canónico normalizado
  int32 digits = 3;              // Decimales
  double point = 4;              // Tamaño del point
  double tick_size = 5;
  double tick_value = 6;
  double min_lot = 7;
  double max_lot = 8;
  double lot_step = 9;
  int32 stop_level = 10;         // Nivel mínimo de stop en points
  optional double contract_size = 11;
  optional string description = 12;
}
```

-- Nuevo mensaje para reporte por cuenta (i3):

```protobuf
// sdk/proto/v1/agent.proto (nuevo)
message SymbolMapping {
  string canonical_symbol = 1;
  string broker_symbol = 2;
  int32 digits = 3;
  double point = 4;
  double tick_size = 5;
  double min_lot = 6;
  double max_lot = 7;
  double lot_step = 8;
  int32 stop_level = 9;
  optional double contract_size = 10;
  // Valores dinámicos (i5): tick_value, margin_required se reportarán en StateSnapshot
}

message AccountSymbolsReport {
  string account_id = 1;                       // Cuenta del EA (master o slave)
  repeated SymbolMapping symbols = 2;          // Lista explícita
  int64 reported_at_ms = 3;                    // Para detectar actualizaciones
}

// Extender AgentMessage
message AgentMessage {
  string agent_id = 1;
  int64 timestamp_ms = 2;
  oneof payload {
    AgentHello hello = 10;
    TradeIntent trade_intent = 11;
    TradeClose trade_close = 12;
    TradeModify trade_modify = 13;
    ExecutionResult execution_result = 14;
    StateSnapshot state_snapshot = 15;
    AgentHeartbeat heartbeat = 16;
    AccountConnected account_connected = 17;
    AccountDisconnected account_disconnected = 18;
    AccountSymbolsReport account_symbols_report = 19; // NEW i3
  }
}
```

- `AgentHello.symbols`: se marca como reservado/deprecado en comentarios del proto; no se usa en i3.

- Validaciones (SDK `domain`):
  - `ValidateSymbolInfo(info *pb.SymbolInfo)` ya existe.
  - Nuevas helpers: `NormalizeCanonical(string)` y `ValidateCanonicalSymbol(string, []string)`.
  - Nuevo `ValidateSymbolMapping(mapping, allowedCanonicals)` (campos requeridos y rangos coherentes).
  - Nuevas interfaces de repositorio para símbolos (ver 3.3).

### 3.1.1 Validaciones y Normalización

#### NormalizeCanonical

Reglas:
1. Uppercase y trim.
2. Remover sufijos de broker conocidos: .m, .i, .raw, .ecn, .pro, .c (case-insensitive).
3. Permitir solo A-Z, 0-9, '/', '-', '_'.
4. Longitud post-normalización: 3–20.

Ejemplos:
| Input | Output |
|---|---|
| "xauusd.m" | "XAUUSD" |
| "EUR/USD" | "EUR/USD" |
| "btc-usd" | "BTC-USD" |
| " Gold.ECN " | "GOLD" |
| "SP500.c" | "SP500" |

#### ValidateSymbolMapping

Reglas:
- Requeridos: broker_symbol y canonical_symbol.
- Canonical en whitelist tras normalización.
- Volúmenes: min_lot>0, max_lot>0, min_lot<=max_lot, lot_step>0.
- Precio: digits>=0, point>0, tick_size>0.
- Stop level: >=0 (0 = sin restricción).
- contract_size (opcional): si presente, >0.

Errores típicos: "broker_symbol is required", "canonical_symbol is required", "canonical_symbol X not in whitelist (normalized: Y)", "min_lot (X) > max_lot (Y)", "lot_step must be > 0", "digits must be >= 0", "point must be > 0", "tick_size must be > 0", "stop_level must be >= 0", "contract_size must be > 0".

### 3.2 Agent (Go) y EAs (MQL4/5)

- Agent enviará `AccountSymbolsReport` por cada cuenta conectada tras el handshake del pipe (o inmediatamente después), con al menos el símbolo de trabajo actual (compat incremental). `AgentHello.symbols` no se usa.
- El Slave EA incluirá en su `handshake` un array `symbols` con uno o más `SymbolMapping` mínimos.

Ejemplo de `handshake` extendido (EA → Agent, JSON):

```json
{
  "type": "handshake",
  "timestamp_ms": 1730250000000,
  "payload": {
    "client_id": "slave_123456",
    "account_id": "123456",
    "broker": "Acme Markets Ltd",
    "role": "slave",
    "version": "0.3.0",
    "symbols": [
      {
        "broker_symbol": "XAUUSD.m",
        "canonical_symbol": "XAUUSD",
        "digits": 2,
        "point": 0.01,
        "min_lot": 0.01,
        "max_lot": 100.0,
        "lot_step": 0.01,
        "stop_level": 30
      }
    ]
  }
}
```

- Agent `PipeHandler` extraerá `symbols` y enviará `AccountSymbolsReport` al Core (uno por cuenta). Si el EA no envía `symbols`, el Agent NO enviará reporte y el sistema operará en modo `miss` hasta que el EA sea actualizado (compatibilidad i3 mediante `unknown_action=warn`).

Semántica de reporte:
- Cada `AccountSymbolsReport` representa la LISTA COMPLETA de símbolos de la cuenta (reemplaza la anterior). Al aplicar se invalidan símbolos previos de esa cuenta y se upsertan los nuevos, usando `reported_at_ms` para idempotencia temporal.

### 3.3 Core (Go)

Componentes nuevos y cambios:
- `CanonicalValidator` (nuevo):
  - Fuente de verdad canónica: lista desde ETCD (config). No hay caché en ETCD.
  - API: `IsValid(symbol)`, `Normalize(symbol)`, `List()`, `Validate(ctx, symbol)` que aplica `unknown_action` (`warn`|`reject`).

- `AccountSymbolResolver` (nuevo):
  - Caché en memoria: `account_id × canonical_symbol → SymbolInfo` con `sync.RWMutex`.
  - Persistencia: PostgreSQL (`SymbolRepository`); escritura fuera del lock (async) y lectura solo para warm-up/lazy.
  - API: `ResolveForAccount(ctx, accountID, canonical) (broker string, info *SymbolInfo, found bool)`, `UpsertMappings(ctx, accountID, []SymbolMapping)` y `InvalidateAccount(ctx, accountID)`.

- `SymbolRepository` (nuevo en `core/internal/repository`):
  - `UpsertAccountMapping(ctx, accountID, mapping SymbolMapping)`
  - `GetAccountMapping(ctx, accountID) (map[canonical]*SymbolInfo, error)`

- `Router` (cambios):
  - En `createExecuteOrders`: validar canónico, luego traducir `intent.Symbol` (canónico) → `broker_symbol` por `TargetAccountId`. Si no existe mapeo, actuar según política: `reject` (criterio de salida i3) o `warn` (compat de transición).
  - En `handleTradeClose`: aplicar el mismo mapeo para `CloseOrder.Symbol`.

### 3.4 Persistencia (PostgreSQL)

Tablas (i3):

```sql
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
  reported_at_ms   BIGINT NOT NULL DEFAULT 0,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (account_id, canonical_symbol)
);
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_account ON echo.account_symbol_map(account_id);
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_canonical ON echo.account_symbol_map(canonical_symbol);
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_broker ON echo.account_symbol_map(account_id, broker_symbol);
```

Notas:
- La lista canónica vive en ETCD (config). No se crea tabla `canonical_symbols` en i3. Una tabla de auditoría/log podría considerarse en futuras iteraciones si se requiere.
- `contract_size` se omite del schema i3 para evitar bloat; podrá evaluarse su inclusión en i4/i5 si se requiere persistirlo.

Upsert idempotente (repositorio):
```sql
INSERT INTO echo.account_symbol_map (
  account_id, canonical_symbol, broker_symbol,
  digits, point, tick_size, min_lot, max_lot, lot_step, stop_level,
  reported_at_ms, updated_at
)
VALUES (...)
ON CONFLICT (account_id, canonical_symbol)
DO UPDATE SET
  broker_symbol = EXCLUDED.broker_symbol,
  digits = EXCLUDED.digits,
  point = EXCLUDED.point,
  tick_size = EXCLUDED.tick_size,
  min_lot = EXCLUDED.min_lot,
  max_lot = EXCLUDED.max_lot,
  lot_step = EXCLUDED.lot_step,
  stop_level = EXCLUDED.stop_level,
  reported_at_ms = EXCLUDED.reported_at_ms,
  updated_at = NOW()
WHERE EXCLUDED.reported_at_ms >= echo.account_symbol_map.reported_at_ms;
```

### 3.5 Configuración (ETCD)

- ETCD es solo para configuración (no caché):
  - `core/canonical_symbols`: lista separada por comas (reemplaza `core/symbol_whitelist`).
  - Prioridad: `core/canonical_symbols` → `core/symbol_whitelist` (warning) → lista vacía.
  - `core/symbols/unknown_action`: `reject`|`warn` (default i3: `warn` para rollout seguro, ver §8).

### 3.6 Concurrencia, persistencia async y warm-up

- Concurrencia: `AccountSymbolResolver` usa `sync.RWMutex` sobre `cache map[accountID]map[canonical]*SymbolInfo`.
- Persistencia async: canal con buffer y worker dedicado con backpressure y telemetría.

```go
type persistRequest struct {
    accountID string
    mappings  []domain.SymbolMapping
    reportedAtMs int64
}

type accountSymbolResolver struct {
    mu          sync.RWMutex
    cache       map[string]map[string]*domain.SymbolInfo
    repo        domain.SymbolRepository
    persistChan chan persistRequest // buffer configurable (p.ej. 1000)
    telemetry   *telemetry.Client
}

func (r *accountSymbolResolver) Start(ctx context.Context) { go r.persistWorker(ctx) }

func (r *accountSymbolResolver) persistWorker(ctx context.Context) {
    for req := range r.persistChan {
        if err := r.repo.UpsertAccountMapping(ctx, req.accountID, req.mappings, req.reportedAtMs); err != nil {
            r.telemetry.Error(ctx, "Failed to persist symbol mapping", err)
        }
    }
}

func (r *accountSymbolResolver) UpsertMappings(ctx context.Context, accountID string, mappings []domain.SymbolMapping, reportedAtMs int64) error {
    r.mu.Lock()
    if r.cache[accountID] == nil { r.cache[accountID] = make(map[string]*domain.SymbolInfo) }
    for _, m := range mappings { r.cache[accountID][m.CanonicalSymbol] = m.ToSymbolInfo() }
    r.mu.Unlock()

    select {
    case r.persistChan <- persistRequest{accountID, mappings, reportedAtMs}:
    case <-time.After(100 * time.Millisecond):
        r.telemetry.Warn(ctx, "Persist channel full, dropping mapping update")
    }
    return nil
}
```

- Warm-up (lazy): en la primera miss por cuenta, intentar cargar mappings desde PostgreSQL. Si no hay datos aún, devolver `miss` y operar bajo `warn` hasta que llegue un reporte.

ResolveForAccount (lazy):
```go
func (r *accountSymbolResolver) ResolveForAccount(ctx context.Context, accountID, canonical string) (string, *domain.SymbolInfo, bool) {
    r.mu.RLock(); accountMap, hit := r.cache[accountID]; r.mu.RUnlock()
    if !hit {
        if mappings, err := r.repo.GetAccountMapping(ctx, accountID); err == nil && len(mappings) > 0 {
            r.mu.Lock(); r.cache[accountID] = mappings; accountMap = mappings; r.mu.Unlock()
            r.telemetry.EchoMetrics().RecordSymbolsLoaded(ctx, "postgres", len(mappings))
        } else {
            return "", nil, false
        }
    }
    info, found := accountMap[canonical]
    if !found { return "", nil, false }
    return info.BrokerSymbol, info, true
}
```

## 4. Especificación de Cambios (archivos y puntos de edición)

### 4.1 SDK (proto y dominio)
- `sdk/proto/v1/agent.proto`:
  - Añadir `SymbolMapping`, `AccountSymbolsReport` (repeated), `reported_at_ms` y extender `AgentMessage` (id 19).
  - Marcar `AgentHello.symbols` como reservado/deprecado (comentario proto) sin uso en i3.
- `sdk/domain/validation.go`:
  - Añadir `NormalizeCanonical` (reglas explícitas: upper, trim, remover sufijos comunes `.M/.I/.RAW/.ECN/.PRO`, filtrar caracteres; ejemplos documentados) y `ValidateCanonicalSymbol`.
  - Añadir `ValidateSymbolMapping` (campos requeridos; rangos; whitelist canónica).
- `sdk/domain/repository.go`:
  - Añadir interfaces `SymbolRepository` y `RepositoryFactory.SymbolRepository()`.
- `sdk/domain/models.go` (opcional):
  - Añadir tipo `SymbolMapping` si facilita repositorio/transformers.

### 4.2 Agent (Go)
- `agent/internal/pipe_manager.go`:
  - En `handleMasterMessage`/`handleSlaveMessage` para `handshake`, extraer `symbols` (array) y encolar `AgentMessage_AccountSymbolsReport` hacia el Core.
- `agent/internal/stream.go`:
  - Mantener `AgentHello.symbols` sin uso (reservado). El reporte es per-account.

### 4.3 Core (Go)
- `core/internal/config.go`:
  - Leer `core/canonical_symbols` con fallback a `core/symbol_whitelist` (warning) y exponer `unknown_action`.
- `core/internal/symbol_validator.go` (nuevo) y `core/internal/symbol_resolver.go` (nuevo).
- `core/internal/repository/symbols_postgres.go` (nuevo): implementación de `SymbolRepository`.
- `core/internal/core.go`:
  - Nuevo handler `handleAccountSymbolsReport` con span, logs y métrica `echo.symbols.reported`.
  - `handleAccountDisconnected`: además de la lógica existente, invocar `symbolResolver.InvalidateAccount(ctx, account_id)` para limpiar la caché por cuenta.
- `core/internal/router.go`:
  - En `createExecuteOrders`: validar canónico con `CanonicalValidator`, traducir símbolo con `AccountSymbolResolver` y asignar `order.Symbol` al broker; actuar según política si falta mapeo.
  - En `handleTradeClose`: idem para `CloseOrder.Symbol`.

Código a intervenir (referencias actuales):

```372:415:/home/kor/go/src/github.com/xKoRx/echo/core/internal/router.go
func (r *Router) createExecuteOrders(ctx context.Context, intent *pb.TradeIntent, tradeID string) []*pb.ExecuteOrder {
    // ...
    order := domain.TradeIntentToExecuteOrder(intent, opts)
    // i3: aquí traducir order.Symbol (canónico) → broker_symbol por TargetAccountId
    // si no hay mapeo: log+métrica y no incluir en resultado
    // ...
}
```

### 4.4 Persistencia (SQL)
- `deploy/postgres`: crear `migrations/i3_symbols.sql` con tablas propuestas en 3.4 (no modificar `setup.sql` histórico de i1).

## 5. Observabilidad

- Métricas (bundle EchoMetrics):
  - `echo.symbols.lookup` (labels: `result=hit|miss`, `account_id`, `canonical`).
  - `echo.symbols.reported` (labels: `account_id`, `count`).
  - `echo.symbols.validate` (labels: `result=ok|reject`, `symbol`).
  - `echo.symbols.loaded` (labels: `source=etcd|postgres|agent_report`, `count`).

Eventos de `symbols.loaded`:
- `etcd`: al cargar lista canónica en boot del Core.
- `postgres`: al realizar warm-up (boot) o lazy load por cuenta.
- `agent_report`: al procesar un `AccountSymbolsReport`.

- Logs estructurados:
  - `Symbol mapping missing` (WARN) con `account_id`, `canonical_symbol`.
  - `Symbol mapping applied` (DEBUG/INFO) con `account_id`, `canonical`, `broker`.

- Trazas:
  - Span `core.handle_account_symbols_report` (procesamiento de reportes).
  - Span alrededor de validación+resolución en `Router`.

## 6. Compatibilidad y Migración

- Backward compatible:
  - Sin `AccountSymbolsReport`, Core sigue operando con whitelist (ETCD) y símbolo original, con métricas `miss` y WARN.
  - Activar `unknown_action=reject` solo cuando 100% de cuentas reporten.

- Rollout (3 fases):
  1) Core i3 en modo compatible (`unknown_action=warn`, mantiene whitelist). Métricas activas.
  2) Despliegue progresivo de Agents/EAs i3 (envían reports). Monitorear `echo.symbols.reported`.
  3) Activar `reject` cuando todas las cuentas reporten (flag ETCD).

## 7. Validación Manual (i3)

1. Conectar Slave EA sin `symbols` en handshake → Core registra cuenta; `lookup: miss`, envíos mantienen símbolo original (modo compatibilidad). Logs WARN presentes.
2. Enviar `AccountSymbolsReport` con `XAUUSD.m → XAUUSD` → `lookup: hit`; `ExecuteOrder.Symbol` llega al EA como `XAUUSD.m`.
3. Master envía `TradeClose` → `CloseOrder.Symbol` mapeado; cierre exitoso.
4. Enviar `canonical` no incluido en `core/canonical_symbols` → rechazo con métrica `validate: reject`.

## 8. Criterios de Aceptación

- [ ] Core traduce `intent.Symbol` (canónico) a `broker_symbol` por cuenta antes de enviar órdenes; sin traducción no se envía (si `unknown_action=reject`).
- [ ] `AccountSymbolsReport` procesado y persistido por cuenta, con actualización idempotente.
- [ ] Métricas `echo.symbols.lookup`, `echo.symbols.reported`, `echo.symbols.validate` activas.
- [ ] Config `core/canonical_symbols` en ETCD y uso preferente sobre `core/symbol_whitelist`.
- [ ] Logs y spans con atributos de símbolo y cuenta.

## 9. Plan de Rollout y Rollback

Estrategia incremental (detallada en §6):
- Fase 1: Core i3 (`warn`).
- Fase 2: Agents/EAs i3 desplegados.
- Fase 3: Activar `reject` cuando 100% reporten.

Rollback:
- Revertir Agents/EAs a i2 (Core i3 sigue con whitelist y compatibilidad).
- Revertir Core a i2 si hay problemas en resolución/persistencia.

## 10. Riesgos y Mitigaciones

| Riesgo | Prob. | Impacto | Mitigación |
|---|---|---|---|
| Mapeo incompleto por cuenta | Media | Medio | Métricas `miss`, warnings y rechazo controlado; fallback temporal a whitelist.
| Inconsistencia canónico vs broker | Baja | Medio | Validación SDK; normalización `NormalizeCanonical`.
| Latencia adicional en lookup | Baja | Bajo | Caché en memoria O(1); warm-up al recibir reportes; watch de ETCD fuera del hot-path.
| Migración de config | Baja | Bajo | Soporte de ambas claves (`symbol_whitelist` y `canonical_symbols`) durante i3.

## 11. Documentación y Comunicación

Actualizar:
- `docs/01-arquitectura-y-roadmap.md`: marcar i3 como "Propuesta en desarrollo" y detallar criterios de salida.
- `docs/ea/*_GUIDE.md`: añadir sección de handshake con `symbols` y ejemplo.
- `deploy/postgres/README.md`: nueva migración i3 para símbolos.

## 12. Notas de Calidad y Principios

- World-class: diseño extensible, validación clara, observabilidad completa.
- Clean Code/SOLID: Validator/Resolver separados, repositorios aislados.
- Modularidad/Escalabilidad: SDK-first (contratos/validaciones); Core orquesta; Agents reportan; caché en memoria.
- Configuración: ETCD solo para config (no caché). Carga única con prioridad clara.
- Observabilidad: logs/metrics/traces vía `sdk/telemetry`, atributos en contexto, spans específicos.
- Testing: por política del proyecto, pruebas formales al completar V1; i3 con validación manual documentada.

Nota: Mongo documentado como posible read-model futuro si escala lo exige; no se habilita en i3.


