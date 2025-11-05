package etcd

import (
	"context"
	"testing"
	"time"
)

// TestSeedEchoConfig_Development siembra configuración de Echo en ETCD para desarrollo (i1).
//
// Uso:
//
//	cd /home/kor/go/src/github.com/xKoRx/echo/sdk/etcd
//	go test -v -run TestSeedEchoConfig_Development
//
// Este test puebla el namespace echo/development con la configuración mínima de i1.
func TestSeedEchoConfig_Development(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cliente para echo/development
	client, err := New(
		WithApp("echo"),
		WithEnv("development"),
	)
	if err != nil {
		t.Fatalf("Failed to create ETCD client: %v", err)
	}
	defer client.Close()

	// Configuración i1 según RFC-003 sección 10
	config := map[string]string{
		// Endpoints
		"endpoints/core_addr":             "192.168.31.211:50051",
		"endpoints/otel/otlp_endpoint":    "192.168.31.60:4317",
		"endpoints/otel/metrics_endpoint": "192.168.31.60:14317",

		// Agent (i1 - nuevas claves para config.go)
		"agent/pipe_prefix":         "echo_",
		"agent/master_accounts":     "2089126182,2089126187", // Masters i0
		"agent/slave_accounts":      "2089126183,2089126186", // Slaves i0
		"agent/retry_enabled":       "true",
		"agent/max_retries":         "3",
		"agent/flush_force":         "false", // Desactivado por defecto (i1)
		"agent/send_queue_size":     "100",
		"agent/reconnect_backoff_s": "5",
		"agent/log_level":           "WARN", // INFO, DEBUG, WARN, ERROR

		// gRPC KeepAlive Server (RFC-003 sección 7)
		"grpc/keepalive/time_s":     "60",
		"grpc/keepalive/timeout_s":  "20",
		"grpc/keepalive/min_time_s": "10",

		// gRPC KeepAlive Client (i1 - Agent)
		"grpc/client_keepalive/time_s":                "60",
		"grpc/client_keepalive/timeout_s":             "20",
		"grpc/client_keepalive/permit_without_stream": "false",

		// Core
		"core/grpc_port":                "50051",
		"core/default_lot_size":         "0.10",   // i0 hardcoded, se mantiene en i1 (deprecated i4)
		"core/dedupe_ttl_minutes":       "60",     // 1 hora
		"core/symbol_whitelist":         "XAUUSD", // i0: solo XAUUSD
		"core/log_level":                "WARN",   // INFO, DEBUG, WARN, ERROR
		"core/specs/default_lot":        "0.10",
		"core/specs/missing_policy":     "reject",
		"core/specs/max_age_ms":         "10000",
		"core/specs/alert_threshold_ms": "8000",
		"core/risk/missing_policy":      "reject",
		"core/risk/cache_ttl_ms":        "5000",

		// Slave Accounts (i0: cuentas demo reales)
		"core/slave_accounts": "2089126183,2089126186",

		"core/canonical_symbols":      "XAUUSD,DAX", // XAUUSD,DAX,EURUSD,GBPUSD,USDCHF,USDJPY
		"core/symbols/unknown_action": "reject",     // reject, warn, error

		// Agent extras
		"agent/canonical_symbols": "XAUUSD,DAX",

		// PostgreSQL
		"postgres/host":           "192.168.31.220",
		"postgres/port":           "5432",
		"postgres/database":       "echo",
		"postgres/user":           "echo_user",
		"postgres/password":       "cascada123",
		"postgres/schema":         "echo",
		"postgres/pool_max_conns": "10",
		"postgres/pool_min_conns": "2",

		// Telemetry
		"telemetry/service_name":    "echo",
		"telemetry/service_version": "1.0.0-i1",
		"telemetry/environment":     "development",

		// Policies (por cuenta - ejemplo)
		// TODO i2+: mover a DB con ETCD watches
		"policy/2089126183/max_spread_points":      "30",
		"policy/2089126183/max_slippage_points":    "20",
		"policy/2089126183/max_delay_ms":           "5000",
		"policy/2089126183/copy_sl_tp":             "false",
		"policy/2089126183/catastrophic_sl_points": "500",

		"policy/2089126186/max_spread_points":      "30",
		"policy/2089126186/max_slippage_points":    "20",
		"policy/2089126186/max_delay_ms":           "5000",
		"policy/2089126186/copy_sl_tp":             "false",
		"policy/2089126186/catastrophic_sl_points": "500",
	}

	// Escribir todas las claves
	for key, value := range config {
		if err := put(ctx, client, key, value); err != nil {
			t.Fatalf("Failed to set %s: %v", key, err)
		}
		t.Logf("✅ Set: %s = %s", key, value)
	}

	t.Logf("✅ Echo i1 development config seeded successfully (%d keys)", len(config))

	// Verificar que se pueden leer
	readKey := "endpoints/core_addr"
	val, err := client.GetVar(ctx, readKey)
	if err != nil {
		t.Fatalf("Failed to read back %s: %v", readKey, err)
	}
	t.Logf("🔍 Verification: %s = %s", readKey, val)
}

// TestSeedEchoConfig_Production siembra configuración de Echo para producción (i1).
//
// IMPORTANTE: Ajustar endpoints reales antes de ejecutar en producción.
func TestSeedEchoConfig_Production(t *testing.T) {
	t.Skip("Skipped by default - enable manually for production seeding")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cliente para echo/production
	client, err := New(
		WithApp("echo"),
		WithEnv("production"),
	)
	if err != nil {
		t.Fatalf("Failed to create ETCD client: %v", err)
	}
	defer client.Close()

	// Configuración production (ajustar según entorno real)
	config := map[string]string{
		// Endpoints
		"endpoints/core_addr":             "192.168.31.211:50051", // Ajustar IP real
		"endpoints/otel/otlp_endpoint":    "192.168.31.60:4317",
		"endpoints/otel/metrics_endpoint": "192.168.31.60:14317",

		// Agent (i1)
		"agent/pipe_prefix":         "echo_",
		"agent/master_accounts":     "PROD_MASTER_1,PROD_MASTER_2", // Ajustar
		"agent/slave_accounts":      "PROD_SLAVE_1,PROD_SLAVE_2",   // Ajustar
		"agent/retry_enabled":       "true",
		"agent/max_retries":         "3",
		"agent/flush_force":         "false",
		"agent/send_queue_size":     "100",
		"agent/reconnect_backoff_s": "5",

		// gRPC KeepAlive Server
		"grpc/keepalive/time_s":     "60",
		"grpc/keepalive/timeout_s":  "20",
		"grpc/keepalive/min_time_s": "10",

		// gRPC KeepAlive Client (i1)
		"grpc/client_keepalive/time_s":                "60",
		"grpc/client_keepalive/timeout_s":             "20",
		"grpc/client_keepalive/permit_without_stream": "false",

		// Core
		"core/grpc_port":          "50051",
		"core/default_lot_size":   "0.10",
		"core/dedupe_ttl_minutes": "60",
		"core/symbol_whitelist":   "XAUUSD",

		// Slave Accounts (ajustar a cuentas reales de producción)
		"core/slave_accounts": "PROD_ACCOUNT_1,PROD_ACCOUNT_2",

		// PostgreSQL (ajustar a DB de producción)
		"postgres/host":           "192.168.31.220",
		"postgres/port":           "5432",
		"postgres/database":       "echo",
		"postgres/user":           "postgres",
		"postgres/schema":         "echo",
		"postgres/pool_max_conns": "20",
		"postgres/pool_min_conns": "5",

		// Telemetry
		"telemetry/service_name":    "echo",
		"telemetry/service_version": "1.0.0-i1",
		"telemetry/environment":     "production",
	}

	for key, value := range config {
		if err := put(ctx, client, key, value); err != nil {
			t.Fatalf("Failed to set %s: %v", key, err)
		}
		t.Logf("✅ Set: %s = %s", key, value)
	}

	t.Logf("✅ Echo i1 production config seeded successfully (%d keys)", len(config))
}

// TestListAllEchoKeys lista todas las claves de Echo en ETCD (útil para debugging).
func TestListAllEchoKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := New(
		WithApp("echo"),
		WithEnv("development"),
	)
	if err != nil {
		t.Fatalf("Failed to create ETCD client: %v", err)
	}
	defer client.Close()

	// Listar todas las claves con prefijo vacío
	keys, err := listAll(ctx, client, "")
	if err != nil {
		t.Fatalf("Failed to list keys: %v", err)
	}

	if len(keys) == 0 {
		t.Log("⚠️  No keys found. Run TestSeedEchoConfig_Development first.")
		return
	}

	t.Logf("📋 Found %d keys in echo/development:", len(keys))
	for key, value := range keys {
		t.Logf("  - %s = %s", key, value)
	}
}

// TestCleanupEchoKeys elimina todas las claves de Echo en development (útil para testing).
func TestCleanupEchoKeys(t *testing.T) {
	t.Skip("Skipped by default - enable manually to cleanup")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := New(
		WithApp("echo"),
		WithEnv("development"),
	)
	if err != nil {
		t.Fatalf("Failed to create ETCD client: %v", err)
	}
	defer client.Close()

	// Obtener todas las claves
	keys, err := listAll(ctx, client, "")
	if err != nil {
		t.Fatalf("Failed to list keys: %v", err)
	}

	// Eliminar cada una
	for key := range keys {
		if err := del(ctx, client, key); err != nil {
			t.Logf("⚠️  Failed to delete %s: %v", key, err)
		} else {
			t.Logf("🗑️  Deleted: %s", key)
		}
	}

	t.Logf("✅ Cleanup completed (%d keys deleted)", len(keys))
}

// setVar es un helper para escribir una clave en ETCD.
func setVar(ctx context.Context, client *Client, key, value string) error {
	return client.SetVar(ctx, key, value)
}

// del es un helper para eliminar una clave en ETCD.
func del(ctx context.Context, client *Client, key string) error {
	return client.DeleteVar(ctx, key)
}
