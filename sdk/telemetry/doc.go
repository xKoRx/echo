// Package telemetry proporciona observabilidad completa para echo mediante los tres pilares:
//
// 1. Logs: Registro estructurado JSON compatible con Loki
// 2. Métricas: OpenTelemetry exportables a Prometheus
// 3. Trazas: Trazado distribuido con OpenTelemetry/Jaeger
//
// Uso básico:
//
//	import (
//	    "context"
//	    "github.com/xKoRx/echo/sdk/telemetry"
//	)
//
//	func main() {
//	    ctx := context.Background()
//
//	    // Inicializar telemetría
//	    client, err := telemetry.New(ctx, "echo-core", "production")
//	    if err != nil {
//	        panic(err)
//	    }
//	    defer client.Shutdown(ctx)
//
//	    // Registrar logs
//	    client.Info(ctx, "Operación completada")
//
//	    // Crear span
//	    ctx, span := client.StartSpan(ctx, "process_trade")
//	    defer span.End()
//
//	    // Registrar métricas
//	    client.RecordCounter(ctx, "trades.processed", 1)
//	}
//
// El paquete sigue las mejores prácticas de observabilidad y es compatible
// con el ecosistema OpenTelemetry estándar.
package telemetry

