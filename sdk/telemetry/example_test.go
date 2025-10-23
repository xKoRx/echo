package telemetry_test

import (
	"context"
	"fmt"
	"time"

	"github.com/xKoRx/echo/sdk/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

// ExampleNew demuestra cómo crear y usar el cliente de telemetría
func ExampleNew() {
	ctx := context.Background()
	
	// Crear cliente
	client, err := telemetry.New(ctx, "echo-example", "development",
		telemetry.WithVersion("0.0.1"),
		telemetry.WithOTLPEndpoint("localhost:4317"),
	)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown(ctx)
	
	// Añadir atributos comunes al contexto
	ctx = telemetry.AppendCommonAttrs(ctx,
		attribute.String("component", "copier"),
	)
	
	// Registrar logs
	client.Info(ctx, "Starting trade copy operation",
		attribute.String("symbol", "XAUUSD"),
	)
	
	// Crear span para trazado
	ctx, span := client.StartSpan(ctx, "copy_trade")
	defer span.End()
	
	// Registrar métrica
	start := time.Now()
	// ... operación ...
	latency := time.Since(start).Milliseconds()
	client.RecordLatency(ctx, "trade.copy", float64(latency),
		attribute.String("result", "success"),
	)
	
	fmt.Println("Telemetry example completed")
	// Output: Telemetry example completed
}

// ExampleClient_RecordCounter demuestra el uso de contadores
func ExampleClient_RecordCounter() {
	ctx := context.Background()
	client, _ := telemetry.New(ctx, "echo-test", "test")
	defer client.Shutdown(ctx)
	
	// Registrar evento
	client.RecordCounter(ctx, "trades.processed", 1,
		attribute.String("master", "MT4-001"),
		attribute.String("slave", "MT5-002"),
	)
	
	fmt.Println("Counter recorded")
	// Output: Counter recorded
}

