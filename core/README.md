# Echo Core

Core es el **orquestador central** del sistema Echo Trade Copier. Actúa como cerebro del sistema, gestionando la replicación de operaciones desde cuentas Master hacia cuentas Slave.

## 📋 Responsabilidades

- **Servidor gRPC bidireccional**: Acepta conexiones de múltiples Agents
- **Validación**: Valida TradeIntents usando whitelist de símbolos
- **Deduplicación**: Previene duplicados usando map in-memory con TTL
- **Transformación**: Convierte TradeIntent → ExecuteOrder (con sizing)
- **Routing**: Distribuye ExecuteOrders a los Agents correspondientes
- **Observabilidad**: Logs estructurados + métricas OTEL + trazas

## 🏗️ Arquitectura

```
┌─────────────┐
│   Agent 1   │◄────┐
└─────────────┘     │
                    │
┌─────────────┐     │    ┌──────────────┐
│   Agent 2   │◄────┼────┤     Core     │
└─────────────┘     │    │              │
                    │    │ - Router     │
┌─────────────┐     │    │ - Dedupe     │
│   Agent N   │◄────┘    │ - Validator  │
└─────────────┘          └──────────────┘
```

## 🚀 Iteración 0 (POC)

### Alcance i0

- ✅ **Servidor gRPC**: Puerto 50051, bidi-streaming
- ✅ **Validación**: Solo símbolo XAUUSD permitido
- ✅ **Dedupe**: Map in-memory, TTL 1 hora
- ✅ **Procesamiento**: Secuencial FIFO (canal buffered)
- ✅ **Lot Size**: Hardcoded 0.10 (sin Money Management)
- ✅ **Broadcast**: Envía ExecuteOrders a TODOS los Agents
- ✅ **Telemetría**: EchoMetrics bundle completo

### Limitaciones i0

- ❌ Sin Money Management (lot size fijo)
- ❌ Sin persistencia (Postgres)
- ❌ Sin configuración dinámica (etcd)
- ❌ Sin routing inteligente (broadcast simple)
- ❌ Sin concurrencia (procesamiento secuencial)
- ❌ Sin reintentos
- ❌ Sin SL/TP offset

## 📦 Estructura

```
core/
├── cmd/
│   └── echo-core/
│       └── main.go              # Entry point
├── internal/
│   ├── core.go                  # Core principal
│   ├── dedupe.go                # Dedupe store in-memory
│   └── router.go                # Router/Processor
├── go.mod
└── README.md
```

## 🔧 Dependencias SDK

El Core **NO reimplementa lógica**, solo usa SDK:

| SDK Package | Uso |
|-------------|-----|
| `sdk/pb/v1` | Tipos proto (TradeIntent, ExecuteOrder, etc.) |
| `sdk/domain` | Validaciones + Transformaciones |
| `sdk/telemetry` | Logs + Métricas + Trazas |
| `sdk/utils` | UUIDv7, timestamps |
| `sdk/grpc` | **NO usado en i0** (gRPC nativo) |

## 🏃 Ejecutar

```bash
# Desde directorio core/
go run cmd/echo-core/main.go

# O compilar binario
go build -o bin/echo-core cmd/echo-core/main.go
./bin/echo-core
```

**Output esperado**:
```
2025-10-26 12:00:00 echo-core v0.1.0 starting...
2025-10-26 12:00:00 INFO Core initialized grpc_port=50051
2025-10-26 12:00:00 INFO gRPC server listening address=:50051
2025-10-26 12:00:00 INFO Router started
echo-core v0.1.0 is running on port 50051. Press Ctrl+C to stop.
```

## 📊 Métricas

Core registra las siguientes métricas (EchoMetrics):

### Counters
- `echo.order.created`: ExecuteOrders creados
- `echo.order.sent`: ExecuteOrders enviados a Agents
- `echo.execution.completed`: Ejecuciones finalizadas (success/error)

### Histograms (TODO i1)
- `echo.latency.core_process`: Latencia procesamiento interno
- `echo.latency.core_to_agent`: Latencia Core → Agent

## 🔬 Logs Estructurados

Todos los logs son JSON estructurado compatible con Loki:

```json
{
  "level": "INFO",
  "msg": "TradeIntent received",
  "trade_id": "01HKQV8Y...",
  "symbol": "XAUUSD",
  "order_side": "BUY",
  "client_id": "master_12345"
}
```

## 🔀 Flujo de Procesamiento

```
1. Agent envía TradeIntent via gRPC stream
   ↓
2. Core recibe → Router encola (canal FIFO)
   ↓
3. Router procesa secuencialmente:
   a. Validar símbolo (XAUUSD)
   b. Dedupe (rechazar duplicados)
   c. Transformar → ExecuteOrder (lot = 0.10)
   d. Broadcast a TODOS los Agents
   ↓
4. Agents reciben ExecuteOrder → envían a Slaves
   ↓
5. Slaves ejecutan → reportan ExecutionResult
   ↓
6. Core recibe ExecutionResult → Métricas
```

## 🧪 Testing (TODO i1)

```bash
# Unit tests
go test ./internal/...

# Integration tests (requiere Agent + EAs)
go test ./...
```

## 🐛 Troubleshooting

### Core no arranca

**Error**: `failed to listen on :50051: address already in use`

**Solución**: Puerto 50051 ya en uso. Cambiar puerto en `DefaultConfig()` o matar proceso.

```bash
# Windows
netstat -ano | findstr :50051
taskkill /PID <PID> /F

# Linux
lsof -i :50051
kill -9 <PID>
```

### Agents no se conectan

**Error**: Logs no muestran "Agent connected"

**Verificar**:
1. Core escuchando en puerto correcto (`INFO gRPC server listening`)
2. Agent configurado con `core_address = "localhost:50051"`
3. Firewall no bloqueando puerto

### TradeIntents rechazados

**Error**: `WARN Invalid symbol, TradeIntent rejected`

**Causa**: Símbolo no está en whitelist (solo XAUUSD en i0)

**Solución**: Verificar que Master EA envía `symbol = "XAUUSD"`

### Duplicados no detectados

**Verificar**:
1. Master EA genera `trade_id` único (UUIDv7)
2. Logs muestran `"Duplicate TradeIntent rejected"` si hay duplicado
3. Dedupe store no lleno (cleanup cada 1 min)

## 🚧 Roadmap

### Iteración 1
- [ ] Persistencia Postgres (órdenes, dedupe)
- [ ] Config desde etcd (whitelist, lot sizes)
- [ ] Money Management central (risk-based sizing)
- [ ] Routing inteligente (config account → slaves)
- [ ] Reintentos con backoff

### Iteración 2
- [ ] Procesamiento concurrente (locks por trade_id)
- [ ] SL/TP con offsets configurables
- [ ] Ventanas de no-ejecución
- [ ] Filtros (spread, slippage, age)

### Iteración 3
- [ ] Health checks + heartbeats
- [ ] Reconciliación de estado
- [ ] Dashboard Grafana
- [ ] CLI de administración

## 📚 Referencias

- [RFC-001: Arquitectura General](../docs/rfcs/RFC-001-architecture.md)
- [RFC-002: Implementación i0](../docs/rfcs/RFC-002-iteration-0-implementation.md)
- [SDK Documentation](../sdk/README.md)
- [Proto Contracts](../sdk/proto/v1/)

## 📄 Licencia

Propiedad de Aranea Labs. Uso interno.

---

**Versión**: 0.1.0 (Iteración 0)  
**Última actualización**: 2025-10-26
