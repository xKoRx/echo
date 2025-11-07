// Package internal contiene configuración del Core cargada desde ETCD.
package internal

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	"github.com/xKoRx/echo/sdk/etcd"
)

// Config configuración del Core (i1).
//
// Cargada desde ETCD en namespace echo/{environment}.
type Config struct {
	// Endpoints
	CoreAddr        string // endpoints/core_addr
	OTLPEndpoint    string // endpoints/otel/otlp_endpoint
	MetricsEndpoint string // endpoints/otel/metrics_endpoint

	// gRPC
	GRPCPort int // core/grpc_port

	// gRPC KeepAlive (RFC-003 sección 7)
	KeepAliveTime    time.Duration // grpc/keepalive/time_s
	KeepAliveTimeout time.Duration // grpc/keepalive/timeout_s
	KeepAliveMinTime time.Duration // grpc/keepalive/min_time_s

	// Core
	DefaultLotSize   float64       // core/default_lot_size (i0 hardcoded) - deprecated i4, usar VolumeGuardPolicy.DefaultLot
	DedupeTTL        time.Duration // core/dedupe_ttl_minutes
	SymbolWhitelist  []string      // core/symbol_whitelist (comma separated) - deprecated i3, usar CanonicalSymbols
	CanonicalSymbols []string      // core/canonical_symbols (comma separated) - NEW i3
	UnknownAction    string        // core/symbols/unknown_action ("warn"|"reject") - NEW i3
	SlaveAccounts    []string      // core/slave_accounts (comma separated)
	VolumeGuard      *domain.VolumeGuardPolicy
	Risk             RiskConfig
	Protocol         ProtocolConfig

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
	LogLevel       string // core/log_level (DEBUG|INFO|WARN|ERROR)
}

// RiskConfig agrupa configuración del servicio de políticas de riesgo.
type RiskConfig struct {
	MissingPolicy string
	CacheTTL      time.Duration
	Engine        FixedRiskEngineConfig
}

// ProtocolConfig agrupa configuración de versionado de handshake.
type ProtocolConfig struct {
	MinVersion       int
	MaxVersion       int
	BlockedVersions  []int
	RequiredFeatures []string
	RetryInterval    time.Duration
	VersionRange     *handshake.VersionRange
}

// FixedRiskEngineConfig agrupa la configuración del motor FixedRisk.
type FixedRiskEngineConfig struct {
	QuoteMaxAge              time.Duration
	MinDistancePoints        float64
	MaxRiskDrift             float64
	DefaultCurrency          string
	EnableCurrencyFallback   bool
	RejectOnMissingTickValue bool
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
		SymbolWhitelist:     []string{"XAUUSD"}, // Deprecated i3, mantener para compatibilidad
		CanonicalSymbols:    []string{"XAUUSD"}, // NEW i3
		UnknownAction:       "warn",             // NEW i3 (default: warn para rollout seguro)
		PostgresPort:        5432,
		PostgresSchema:      "echo",
		PostgresPoolMaxConn: 10,
		PostgresPoolMinConn: 2,
		ServiceName:         "echo-core",
		ServiceVersion:      "1.0.0-i1",
		Environment:         env,
		LogLevel:            "INFO",
		VolumeGuard: &domain.VolumeGuardPolicy{
			OnMissingSpec:  domain.VolumeGuardMissingSpecReject,
			MaxSpecAge:     10 * time.Second,
			AlertThreshold: 8 * time.Second,
			DefaultLot:     0.10,
		},
		Risk: RiskConfig{
			MissingPolicy: "reject",
			CacheTTL:      5 * time.Second,
			Engine: FixedRiskEngineConfig{
				QuoteMaxAge:              750 * time.Millisecond,
				MinDistancePoints:        5,
				MaxRiskDrift:             0.02,
				DefaultCurrency:          "USD",
				EnableCurrencyFallback:   false,
				RejectOnMissingTickValue: true,
			},
		},
		Protocol: ProtocolConfig{
			MinVersion:       handshake.ProtocolVersionV1,
			MaxVersion:       handshake.ProtocolVersionV2,
			BlockedVersions:  []int{},
			RequiredFeatures: []string{},
			RetryInterval:    5 * time.Minute,
		},
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
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/specs/default_lot", ""); err == nil && val != "" {
		if lotSize, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.VolumeGuard.DefaultLot = lotSize
			cfg.DefaultLotSize = lotSize
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/default_lot_size", ""); err == nil && val != "" {
		if lotSize, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.DefaultLotSize = lotSize
			if cfg.VolumeGuard != nil {
				cfg.VolumeGuard.DefaultLot = lotSize
			}
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/dedupe_ttl_minutes", ""); err == nil && val != "" {
		if minutes, err := strconv.Atoi(val); err == nil {
			cfg.DedupeTTL = time.Duration(minutes) * time.Minute
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/specs/missing_policy", ""); err == nil && val != "" {
		switch strings.ToLower(val) {
		case string(domain.VolumeGuardMissingSpecReject):
			cfg.VolumeGuard.OnMissingSpec = domain.VolumeGuardMissingSpecReject
		default:
			return nil, fmt.Errorf("unsupported core/specs/missing_policy: %s", val)
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/specs/max_age_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(val); err == nil {
			cfg.VolumeGuard.MaxSpecAge = time.Duration(ms) * time.Millisecond
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/specs/alert_threshold_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(val); err == nil {
			cfg.VolumeGuard.AlertThreshold = time.Duration(ms) * time.Millisecond
		}
	}
	// i3: Leer canonical_symbols (prioridad) con fallback a symbol_whitelist (compatibilidad)
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/canonical_symbols", ""); err == nil && val != "" {
		cfg.CanonicalSymbols = strings.Split(val, ",")
		// Trim spaces
		for i := range cfg.CanonicalSymbols {
			cfg.CanonicalSymbols[i] = strings.TrimSpace(cfg.CanonicalSymbols[i])
		}
	} else if val, err := etcdClient.GetVarWithDefault(ctx, "core/symbol_whitelist", ""); err == nil && val != "" {
		// Fallback a symbol_whitelist (compatibilidad i3)
		cfg.CanonicalSymbols = strings.Split(val, ",")
		for i := range cfg.CanonicalSymbols {
			cfg.CanonicalSymbols[i] = strings.TrimSpace(cfg.CanonicalSymbols[i])
		}
		// Mantener también en SymbolWhitelist para compatibilidad
		cfg.SymbolWhitelist = cfg.CanonicalSymbols
	}

	// Mantener lectura de symbol_whitelist para compatibilidad (si no se lee canonical_symbols)
	if len(cfg.CanonicalSymbols) == 0 {
		if val, err := etcdClient.GetVarWithDefault(ctx, "core/symbol_whitelist", ""); err == nil && val != "" {
			cfg.SymbolWhitelist = strings.Split(val, ",")
			for i := range cfg.SymbolWhitelist {
				cfg.SymbolWhitelist[i] = strings.TrimSpace(cfg.SymbolWhitelist[i])
			}
			// Usar también como canonical para compatibilidad
			cfg.CanonicalSymbols = cfg.SymbolWhitelist
		}
	}

	// i3: Leer unknown_action
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/symbols/unknown_action", ""); err == nil && val != "" {
		if val == "warn" || val == "reject" {
			cfg.UnknownAction = val
		}
	}

	if val, err := etcdClient.GetVarWithDefault(ctx, "core/slave_accounts", ""); err == nil && val != "" {
		cfg.SlaveAccounts = strings.Split(val, ",")
		// Trim spaces
		for i := range cfg.SlaveAccounts {
			cfg.SlaveAccounts[i] = strings.TrimSpace(cfg.SlaveAccounts[i])
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/missing_policy", ""); err == nil && val != "" {
		switch strings.ToLower(val) {
		case "reject":
			cfg.Risk.MissingPolicy = "reject"
		case "warn":
			cfg.Risk.MissingPolicy = "warn"
		default:
			return nil, fmt.Errorf("unsupported core/risk/missing_policy: %s", val)
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/cache_ttl_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(val); err == nil {
			cfg.Risk.CacheTTL = time.Duration(ms) * time.Millisecond
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/quote_max_age_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(val); err == nil {
			cfg.Risk.Engine.QuoteMaxAge = time.Duration(ms) * time.Millisecond
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/min_distance_points", ""); err == nil && val != "" {
		if points, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.Risk.Engine.MinDistancePoints = points
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/max_risk_drift_pct", ""); err == nil && val != "" {
		if drift, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.Risk.Engine.MaxRiskDrift = drift
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/default_currency", ""); err == nil && val != "" {
		cfg.Risk.Engine.DefaultCurrency = strings.ToUpper(strings.TrimSpace(val))
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/enable_currency_fallback", ""); err == nil && val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			cfg.Risk.Engine.EnableCurrencyFallback = enabled
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/risk/reject_on_missing_tick_value", ""); err == nil && val != "" {
		if reject, err := strconv.ParseBool(val); err == nil {
			cfg.Risk.Engine.RejectOnMissingTickValue = reject
		}
	}

	// Protocol versioning (i5)
	protocolMin := cfg.Protocol.MinVersion
	protocolMax := cfg.Protocol.MaxVersion
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/protocol/min_version", ""); err == nil && val != "" {
		if min, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			protocolMin = min
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/protocol/max_version", ""); err == nil && val != "" {
		if max, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			protocolMax = max
		}
	}
	blockedVersions := make([]int, 0)
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/protocol/blocked_versions", ""); err == nil && val != "" {
		parts := strings.Split(val, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if version, err := strconv.Atoi(part); err == nil {
				blockedVersions = append(blockedVersions, version)
			} else {
				return nil, fmt.Errorf("invalid blocked version '%s': %w", part, err)
			}
		}
	}
	requiredFeatures := cfg.Protocol.RequiredFeatures
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/protocol/required_features", ""); err == nil && val != "" {
		parts := strings.Split(val, ",")
		features := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			features = append(features, trimmed)
		}
		requiredFeatures = features
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/protocol/retry_interval_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && ms > 0 {
			cfg.Protocol.RetryInterval = time.Duration(ms) * time.Millisecond
		}
	}
	versionRange, err := handshake.NewVersionRange(protocolMin, protocolMax, blockedVersions)
	if err != nil {
		return nil, fmt.Errorf("invalid core protocol range: %w", err)
	}
	cfg.Protocol.MinVersion = protocolMin
	cfg.Protocol.MaxVersion = protocolMax
	cfg.Protocol.BlockedVersions = blockedVersions
	cfg.Protocol.RequiredFeatures = requiredFeatures
	cfg.Protocol.VersionRange = versionRange

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

	if val, err := etcdClient.GetVarWithDefault(ctx, "core/log_level", ""); err == nil && val != "" {
		level := strings.ToUpper(strings.TrimSpace(val))
		switch level {
		case "DEBUG", "INFO", "WARN", "ERROR":
			cfg.LogLevel = level
		}
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
	if cfg.VolumeGuard == nil {
		return nil, fmt.Errorf("volume guard policy must be configured")
	}
	if err := cfg.VolumeGuard.Validate(); err != nil {
		return nil, fmt.Errorf("invalid volume guard policy: %w", err)
	}
	if cfg.Risk.MissingPolicy != "reject" {
		return nil, fmt.Errorf("invalid risk missing policy: %s", cfg.Risk.MissingPolicy)
	}
	if cfg.Risk.CacheTTL <= 0 {
		cfg.Risk.CacheTTL = 5 * time.Second
	}
	if cfg.Protocol.VersionRange == nil {
		return nil, fmt.Errorf("protocol version range not configured")
	}
	if cfg.Protocol.RetryInterval <= 0 {
		cfg.Protocol.RetryInterval = 5 * time.Minute
	}
	if cfg.Protocol.MinVersion > cfg.Protocol.MaxVersion {
		return nil, fmt.Errorf("protocol min_version %d greater than max_version %d", cfg.Protocol.MinVersion, cfg.Protocol.MaxVersion)
	}
	if cfg.Protocol.RequiredFeatures == nil {
		cfg.Protocol.RequiredFeatures = []string{}
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
