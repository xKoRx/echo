package semconv_test

import (
	"fmt"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// helper para imprimir valores respetando tipo
func printAttr(kv attribute.KeyValue) {
	switch string(kv.Key) {
	case "http.status_code", "http.duration_ms", "document.size":
		fmt.Printf("%s: %v\n", kv.Key, kv.Value.AsInt64())
	case "document.success", "success":
		fmt.Printf("%s: %v\n", kv.Key, kv.Value.AsBool())
	default:
		fmt.Printf("%s: %v\n", kv.Key, kv.Value.AsString())
	}
}

// ExampleLogs muestra cómo utilizar las convenciones semánticas para logs
func ExampleLogs() {
	attrs := []attribute.KeyValue{
		semconv.Logs.Feature.String("Authentication"),
		semconv.Logs.Event.String("login_attempt"),
		attribute.String("user_id", "user123"),
		attribute.Bool("success", true),
	}
	for _, attr := range attrs {
		printAttr(attr)
	}
	// Output:
	// feature: Authentication
	// event: login_attempt
	// user_id: user123
	// success: true
}

// ExampleHTTP muestra cómo utilizar las convenciones semánticas para HTTP
func ExampleHTTP() {
	attrs := []attribute.KeyValue{
		semconv.HTTP.Method.String("GET"),
		semconv.HTTP.Path.String("/api/v1/products"),
		semconv.HTTP.StatusCode.Int(200),
		semconv.HTTP.DurationMs.Int(125),
	}
	for _, attr := range attrs {
		printAttr(attr)
	}
	// Output:
	// http.method: GET
	// http.path: /api/v1/products
	// http.status_code: 200
	// http.duration_ms: 125
}

// ExampleDocument muestra cómo utilizar las convenciones semánticas para documentos
func ExampleDocument() {
	attrs := []attribute.KeyValue{
		semconv.Document.Path.String("/storage/documents/"),
		semconv.Document.Filename.String("report.pdf"),
		semconv.Document.Size.Int(1024567),
		semconv.Document.Type.String("pdf"),
		semconv.Document.Success.Bool(true),
	}
	for _, attr := range attrs {
		printAttr(attr)
	}
	// Output:
	// document.path: /storage/documents/
	// document.filename: report.pdf
	// document.size: 1024567
	// document.type: pdf
	// document.success: true
}

// ExampleMetrics muestra cómo utilizar las convenciones semánticas para métricas
func ExampleMetrics() {
	attrs := []attribute.KeyValue{
		semconv.Metrics.Service.String("order-service"),
		semconv.Metrics.Action.String("process"),
		semconv.Metrics.Status.String("success"),
		semconv.Metrics.Exchange.String("binance"),
		semconv.Metrics.Symbol.String("BTCUSDT"),
	}
	for _, attr := range attrs {
		printAttr(attr)
	}
	// Output:
	// service: order-service
	// action: process
	// status: success
	// exchange: binance
	// symbol: BTCUSDT
}
