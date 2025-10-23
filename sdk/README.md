# SDK - Librería Transversal

El **SDK** de echo contiene todos los componentes **reutilizables** entre core, agent y futuros clientes custom:

- **proto**: Contratos gRPC
- **pb**: Código Go generado
- **telemetry**: Observabilidad (logs, métricas, trazas)
- **ipc**: Helpers para Named Pipes
- **domain**: Entidades del dominio
- **contracts**: Interfaces comunes

## Estructura

```
sdk/
├── proto/                  # Archivos .proto
│   ├── v1/
│   │   ├── common.proto
│   │   ├── trade.proto
│   │   └── agent.proto
│   ├── buf.yaml
│   ├── buf.gen.yaml
│   └── generate.sh
├── pb/                     # Código generado
│   └── v1/
├── telemetry/              # Observabilidad
│   ├── client.go
│   ├── logs.go
│   ├── metrics.go
│   ├── traces.go
│   └── README.md
├── ipc/                    # Named Pipes helpers
├── domain/                 # Entidades
├── contracts/              # Interfaces
└── README.md
```

## Uso

### Proto Generation

```bash
cd sdk/proto
./generate.sh

# O desde raíz:
make proto
```

### Telemetry

```go
import "github.com/xKoRx/echo/sdk/telemetry"

client, err := telemetry.New(ctx, "my-service", "production")
if err != nil {
    panic(err)
}
defer client.Shutdown(ctx)

client.Info(ctx, "Operation completed")
```

Ver [telemetry/README.md](telemetry/README.md) para más detalles.

### IPC Helpers

```go
import "github.com/xKoRx/echo/sdk/ipc"

// TODO: Implementar en iteración 0
```

### Domain

```go
import "github.com/xKoRx/echo/sdk/domain"

// TODO: Definir entidades en iteración 0
```

## Publicación

El SDK está diseñado para ser publicable como módulo independiente:

```bash
go get github.com/xKoRx/echo/sdk@v0.1.0
```

## Versionado

Sigue **SemVer**:

- `v0.x.x`: Iteraciones MVP (breaking changes permitidos)
- `v1.0.0`: Primera versión estable
- `v1.x.x`: Backward compatible

## Tests

```bash
cd sdk
go test ./...
```

## Dependencias

- OpenTelemetry (telemetry)
- gRPC (proto/pb)
- etcd client (config, futuro)

## Próximos Pasos

- [ ] Completar proto V1
- [ ] Implementar IPC helpers
- [ ] Definir domain entities
- [ ] Documentar contracts

