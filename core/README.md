# Core - Servicio Orquestador

El **core** es el cerebro de echo, responsable de:

- Orquestación de copiado de trades
- Money Management centralizado
- Aplicación de políticas (ventanas, tolerancias, límites)
- Reconciliación de estado
- Deduplicación idempotente
- Servidor gRPC bidireccional

## Estructura

```
core/
├── cmd/
│   └── echo-core/
│       └── main.go         # Entry point
├── internal/
│   ├── engine/             # Motor de copia
│   ├── policy/             # Evaluación de políticas
│   ├── state/              # Reconciliación y dedupe
│   ├── repository/         # Acceso a Postgres
│   └── grpc/               # Servidor gRPC
├── pkg/
│   └── config/             # Config exportable
└── README.md
```

## Responsabilidades

### Engine de Replicación

- Recibe `TradeIntent` desde agents (masters)
- Aplica Money Management por cuenta×estrategia
- Valida contra políticas activas
- Distribuye `ExecuteOrder` a agents (slaves)

### Policy Manager

- Ventanas de no-ejecución (carga desde etcd)
- Tolerancias (spread, slippage, delay)
- Límites (DD diario, riesgo por trade)
- SL catastrófico configurable

### State Manager

- Mantiene estado de posiciones por cuenta
- Deduplicación por `trade_id` (UUIDv7)
- Reconciliación periódica con snapshots de agents

### gRPC Server

- Streaming bidireccional con agents
- Health checks
- Config updates vía streaming

## Configuración

El core lee config desde **etcd**:

```yaml
# Ejemplo de keys en etcd
/echo/core/policy/master_accounts: ["MT4-001", "MT4-002"]
/echo/core/policy/spread_max: 5.0
/echo/core/policy/slippage_max: 3.0
/echo/core/database/postgres_url: "postgres://..."
```

## Ejecución

```bash
cd core
go run ./cmd/echo-core

# O compilado:
go build -o echo-core ./cmd/echo-core
./echo-core
```

## Tests

```bash
cd core
go test -v ./...
```

## Dependencias

- `github.com/xKoRx/echo/sdk`: Proto, telemetry, domain
- `google.golang.org/grpc`: gRPC server
- PostgreSQL: Estado y políticas
- etcd: Configuración live

## Próximos Pasos (Iteración 0)

- [ ] Implementar engine básico
- [ ] Servidor gRPC bidi funcional
- [ ] Deduplicación en memoria
- [ ] Tests del flujo happy path

