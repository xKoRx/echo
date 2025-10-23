# Telemetry Package

Paquete de observabilidad completa para **echo**, implementando los tres pilares:

- **Logs**: slog con JSON estructurado
- **Métricas**: OpenTelemetry → Prometheus
- **Trazas**: OpenTelemetry → Jaeger

## Uso Básico

```go
ctx := context.Background()

// Inicializar
client, err := telemetry.New(ctx, "echo-core", "production",
    telemetry.WithVersion("1.0.0"),
    telemetry.WithOTLPEndpoint("localhost:4317"),
)
if err != nil {
    panic(err)
}
defer client.Shutdown(ctx)

// Logs
client.Info(ctx, "Trade copied successfully")

// Métricas
client.RecordCounter(ctx, "trades.copied", 1)

// Trazas
ctx, span := client.StartSpan(ctx, "copy_trade")
defer span.End()
```

## Atributos en Contexto

Evita repetir atributos usando el contexto:

```go
// Al inicio de un request/operación
ctx = telemetry.AppendCommonAttrs(ctx,
    attribute.String("component", "engine"),
)

ctx = telemetry.AppendEventAttrs(ctx,
    attribute.String("trade_id", "abc-123"),
)

// Luego, en cualquier lugar:
client.Info(ctx, "Processing") // Los atributos se incluyen automáticamente
```

## Tests

```bash
go test ./...
```

## Arquitectura

- **client.go**: Cliente unificado
- **logs.go**: Wrapper sobre slog
- **metrics.go**: Helpers para métricas OTEL
- **traces.go**: Helpers para spans OTEL
- **context.go**: Gestión de atributos en contexto
- **config.go**: Configuración y options pattern

## Diferencias con xKoRx/sdk

Esta es una versión **limpia desde cero** sin la deuda técnica de la SDK original:

- Sin dependencias de zeromq
- Sin bundles pre-configurados (se añaden según necesidad)
- API simplificada
- Go 1.25 idiomático

