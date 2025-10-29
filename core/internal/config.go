// Package internal contiene configuración del Core cargada desde ETCD.
package internal

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xKoRx/echo/sdk/etcd"
)

// Config configuración del Core (i1).
//
// Cargada desde ETCD en namespace echo/{environment}.
type Config struct {
	// Endpoints
	CoreAddr         string // endpoints/core_addr
	OTLPEndpoint     string // endpoints/otel/otlp_endpoint
	MetricsEndpoint  string // endpoints/otel/metrics_endpoint

	// gRPC
	GRPCPort int // core/grpc_port

	// gRPC KeepAlive (RFC-003 sección 7)
	KeepAliveTime    time.Duration // grpc/keepalive/time_s
	KeepAliveTimeout time.Duration // grpc/keepalive/timeout_s
	KeepAliveMinTime time.Duration // grpc/keepalive/min_time_s

	// Core
	DefaultLotSize      float64       // core/default_lot_size (i0 hardcoded)
	DedupeTTL           time.Duration // core/dedupe_ttl_minutes
	SymbolWhitelist     []string      // core/symbol_whitelist (comma separated)
	SlaveAccounts       []string      // core/slave_accounts (comma separated)

	// PostgreSQL
	PostgresHost        string // postgres/host
	PostgresPort        int    // postgres/port
	PostgresDatabase    string // postgres/database
	PostgresUser        string // postgres/user
	PostgresPassword    string // postgres/password (si se almacena, mejor usar secret manager)
	PostgresSchema      string // postgres/schema
	PostgresPoolMaxConn int    // postgres/pool_max_conns
	PostgresPoolMinConn int    // postgres/pool_min_conns

	// Telemetry
	ServiceName    string // telemetry/service_name
	ServiceVersion string // telemetry/service_version
	Environment    string // telemetry/environment
}

// LoadConfig carga configuración desde ETCD.
//
// Environment se determina desde variable de entorno ENV (default: development).
//
// Uso:
//
//	cfg, err := internal.LoadConfig(ctx)
//	if err != nil {
//	    return err
//	}
func LoadConfig(ctx context.Context) (*Config, error) {
	// Obtener environment desde ENV (excepción aprobada)
	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	// Crear cliente ETCD para app=echo, env={development|production}
	etcdClient, err := etcd.New(
		etcd.WithApp("echo"),
		etcd.WithEnv(env),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ETCD client: %w", err)
	}
	defer etcdClient.Close()

	// Crear config con defaults
	cfg := &Config{
		// Defaults (sobrescritos por ETCD si existen)
		GRPCPort:            50051,
		KeepAliveTime:       60 * time.Second,
		KeepAliveTimeout:    20 * time.Second,
		KeepAliveMinTime:    10 * time.Second,
		DefaultLotSize:      0.10,
		DedupeTTL:           60 * time.Minute,
		SymbolWhitelist:     []string{"XAUUSD"},
		PostgresPort:        5432,
		PostgresSchema:      "echo",
		PostgresPoolMaxConn: 10,
		PostgresPoolMinConn: 2,
		ServiceName:         "echo-core",
		ServiceVersion:      "1.0.0-i1",
		Environment:         env,
	}

	// Cargar endpoints
	if val, err := etcdClient.GetVarWithDefault(ctx, "endpoints/core_addr", ""); err == nil && val != "" {
		cfg.CoreAddr = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "endpoints/otel/otlp_endpoint", ""); err == nil && val != "" {
		cfg.OTLPEndpoint = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "endpoints/otel/metrics_endpoint", ""); err == nil && val != "" {
		cfg.MetricsEndpoint = val
	}

	// Cargar gRPC
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/grpc_port", ""); err == nil && val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.GRPCPort = port
		}
	}

	// Cargar KeepAlive
	if val, err := etcdClient.GetVarWithDefault(ctx, "grpc/keepalive/time_s", ""); err == nil && val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			cfg.KeepAliveTime = time.Duration(seconds) * time.Second
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "grpc/keepalive/timeout_s", ""); err == nil && val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			cfg.KeepAliveTimeout = time.Duration(seconds) * time.Second
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "grpc/keepalive/min_time_s", ""); err == nil && val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			cfg.KeepAliveMinTime = time.Duration(seconds) * time.Second
		}
	}

	// Cargar Core
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/default_lot_size", ""); err == nil && val != "" {
		if lotSize, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.DefaultLotSize = lotSize
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/dedupe_ttl_minutes", ""); err == nil && val != "" {
		if minutes, err := strconv.Atoi(val); err == nil {
			cfg.DedupeTTL = time.Duration(minutes) * time.Minute
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/symbol_whitelist", ""); err == nil && val != "" {
		cfg.SymbolWhitelist = strings.Split(val, ",")
		// Trim spaces
		for i := range cfg.SymbolWhitelist {
			cfg.SymbolWhitelist[i] = strings.TrimSpace(cfg.SymbolWhitelist[i])
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/slave_accounts", ""); err == nil && val != "" {
		cfg.SlaveAccounts = strings.Split(val, ",")
		// Trim spaces
		for i := range cfg.SlaveAccounts {
			cfg.SlaveAccounts[i] = strings.TrimSpace(cfg.SlaveAccounts[i])
		}
	}

	// Cargar PostgreSQL
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/host", ""); err == nil && val != "" {
		cfg.PostgresHost = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/port", ""); err == nil && val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.PostgresPort = port
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/database", ""); err == nil && val != "" {
		cfg.PostgresDatabase = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/user", ""); err == nil && val != "" {
		cfg.PostgresUser = val
	}
	// Password: en i1 asumir password vacío (trusted auth), en i2+ usar secret manager
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/password", ""); err == nil && val != "" {
		cfg.PostgresPassword = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/schema", ""); err == nil && val != "" {
		cfg.PostgresSchema = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/pool_max_conns", ""); err == nil && val != "" {
		if maxConns, err := strconv.Atoi(val); err == nil {
			cfg.PostgresPoolMaxConn = maxConns
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "postgres/pool_min_conns", ""); err == nil && val != "" {
		if minConns, err := strconv.Atoi(val); err == nil {
			cfg.PostgresPoolMinConn = minConns
		}
	}

	// Cargar Telemetry
	if val, err := etcdClient.GetVarWithDefault(ctx, "telemetry/service_name", ""); err == nil && val != "" {
		cfg.ServiceName = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "telemetry/service_version", ""); err == nil && val != "" {
		cfg.ServiceVersion = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "telemetry/environment", ""); err == nil && val != "" {
		cfg.Environment = val
	}

	// Validar configuración mínima requerida
	if cfg.PostgresHost == "" {
		return nil, fmt.Errorf("postgres/host not configured in ETCD")
	}
	if cfg.PostgresDatabase == "" {
		return nil, fmt.Errorf("postgres/database not configured in ETCD")
	}
	if cfg.PostgresUser == "" {
		return nil, fmt.Errorf("postgres/user not configured in ETCD")
	}
	if len(cfg.SlaveAccounts) == 0 {
		return nil, fmt.Errorf("core/slave_accounts not configured in ETCD")
	}

	return cfg, nil
}

// PostgresConnStr retorna el connection string de PostgreSQL.
//
// Formato: postgres://user:password@host:port/database?sslmode=disable
func (c *Config) PostgresConnStr() string {
	// En i1 asumimos sslmode=disable para desarrollo local
	// TODO i2+: configurar SSL en producción
	password := c.PostgresPassword
	if password != "" {
		password = ":" + password
	}
	return fmt.Sprintf("postgres://%s%s@%s:%d/%s?sslmode=disable",
		c.PostgresUser,
		password,
		c.PostgresHost,
		c.PostgresPort,
		c.PostgresDatabase,
	)
}

