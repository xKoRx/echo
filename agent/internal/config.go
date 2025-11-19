// Package internal contiene configuración del Agent cargada desde ETCD.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xKoRx/echo/sdk/etcd"
)

const (
	minClientHeartbeatSeconds = 1
	maxClientHeartbeatSeconds = 5
)

// Config configuración del Agent (i1).
//
// Cargada desde ETCD en namespace echo/{environment}.
type Config struct {
	// Core
	CoreAddress string // endpoints/core_addr

	// Catálogo de símbolos canónicos (compartido con Core)
	CanonicalSymbols []string

	// Pipes
	PipePrefix     string   // agent/pipe_prefix
	MasterAccounts []string // agent/master_accounts (comma separated)
	SlaveAccounts  []string // agent/slave_accounts (comma separated)

	// Agent
	AgentID          string        // agent/agent_id (persistente para ownership)
	RetryEnabled     bool          // agent/retry_enabled
	MaxRetries       int           // agent/max_retries
	FlushForce       bool          // agent/flush_force (solo benchmark)
	SendQueueSize    int           // agent/send_queue_size
	ReconnectBackoff time.Duration // agent/reconnect_backoff_s

	// gRPC KeepAlive (RFC-003 sección 7)
	KeepAliveTime       time.Duration // grpc/client_keepalive/time_s
	KeepAliveTimeout    time.Duration // grpc/client_keepalive/timeout_s
	PermitWithoutStream bool          // grpc/client_keepalive/permit_without_stream

	// Handshake protocolo
	ProtocolMinVersion  int
	ProtocolMaxVersion  int
	ProtocolAllowLegacy bool

	// Delivery/Ack (i17)
	Delivery DeliveryConfig

	// Telemetry
	ServiceName     string // telemetry/service_name
	ServiceVersion  string // telemetry/service_version
	Environment     string // telemetry/environment
	OTLPEndpoint    string // endpoints/otel/otlp_endpoint
	MetricsEndpoint string // endpoints/otel/metrics_endpoint
	LogLevel        string // agent/log_level (INFO, DEBUG, WARN, ERROR)
}

// DeliveryConfig define parámetros locales del ledger de comandos (i17).
type DeliveryConfig struct {
	AckTimeout   time.Duration
	RetryBackoff []time.Duration
	MaxRetries   int
	LedgerPath   string
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

	// Obtener HOST_KEY para agent_id (excepción aprobada)
	hostKey := os.Getenv("HOST_KEY")
	if hostKey == "" {
		// Fallback: usar hostname
		if hostname, err := os.Hostname(); err == nil {
			hostKey = hostname
		} else {
			hostKey = "unknown"
		}
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
		PipePrefix:          "echo_",
		AgentID:             fmt.Sprintf("agent_%s", hostKey),
		RetryEnabled:        true,
		MaxRetries:          3,
		FlushForce:          false,
		SendQueueSize:       100,
		ReconnectBackoff:    5 * time.Second,
		KeepAliveTime:       5 * time.Second,
		KeepAliveTimeout:    3 * time.Second,
		PermitWithoutStream: false,
		ServiceName:         "echo-agent",
		ServiceVersion:      "1.0.0-i1",
		Environment:         env,
		LogLevel:            "INFO", // Default
		CanonicalSymbols:    []string{"XAUUSD"},
		ProtocolMinVersion:  1,
		ProtocolMaxVersion:  3,
		ProtocolAllowLegacy: true,
		Delivery: DeliveryConfig{
			AckTimeout:   150 * time.Millisecond,
			RetryBackoff: []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond},
			MaxRetries:   100,
		},
	}
	cfg.Delivery.LedgerPath = defaultLedgerPath(cfg.AgentID)
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/canonical_symbols", ""); err == nil && val != "" {
		symbols := strings.Split(val, ",")
		for i := range symbols {
			symbols[i] = strings.TrimSpace(symbols[i])
		}
		cfg.CanonicalSymbols = symbols
	} else if val, err := etcdClient.GetVarWithDefault(ctx, "core/symbol_whitelist", ""); err == nil && val != "" {
		symbols := strings.Split(val, ",")
		for i := range symbols {
			symbols[i] = strings.TrimSpace(symbols[i])
		}
		cfg.CanonicalSymbols = symbols
	}

	// Cargar endpoints
	if val, err := etcdClient.GetVarWithDefault(ctx, "endpoints/core_addr", ""); err == nil && val != "" {
		cfg.CoreAddress = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "endpoints/otel/otlp_endpoint", ""); err == nil && val != "" {
		cfg.OTLPEndpoint = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "endpoints/otel/metrics_endpoint", ""); err == nil && val != "" {
		cfg.MetricsEndpoint = val
	}

	// Cargar Agent
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/pipe_prefix", ""); err == nil && val != "" {
		cfg.PipePrefix = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/agent_id", ""); err == nil && val != "" {
		cfg.AgentID = val
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/retry_enabled", ""); err == nil && val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			cfg.RetryEnabled = enabled
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/max_retries", ""); err == nil && val != "" {
		if maxRetries, err := strconv.Atoi(val); err == nil {
			cfg.MaxRetries = maxRetries
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/flush_force", ""); err == nil && val != "" {
		if flush, err := strconv.ParseBool(val); err == nil {
			cfg.FlushForce = flush
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/send_queue_size", ""); err == nil && val != "" {
		if queueSize, err := strconv.Atoi(val); err == nil {
			cfg.SendQueueSize = queueSize
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/reconnect_backoff_s", ""); err == nil && val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			cfg.ReconnectBackoff = time.Duration(seconds) * time.Second
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/master_accounts", ""); err == nil && val != "" {
		cfg.MasterAccounts = strings.Split(val, ",")
		// Trim spaces
		for i := range cfg.MasterAccounts {
			cfg.MasterAccounts[i] = strings.TrimSpace(cfg.MasterAccounts[i])
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/slave_accounts", ""); err == nil && val != "" {
		cfg.SlaveAccounts = strings.Split(val, ",")
		// Trim spaces
		for i := range cfg.SlaveAccounts {
			cfg.SlaveAccounts[i] = strings.TrimSpace(cfg.SlaveAccounts[i])
		}
	}

	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/protocol/min_version", ""); err == nil && val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			cfg.ProtocolMinVersion = v
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/protocol/max_version", ""); err == nil && val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			cfg.ProtocolMaxVersion = v
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/protocol/allow_legacy", ""); err == nil && val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.ProtocolAllowLegacy = b
		}
	}

	// Delivery overrides
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/delivery/ack_timeout_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && ms > 0 {
			cfg.Delivery.AckTimeout = time.Duration(ms) * time.Millisecond
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/delivery/retry_backoff_ms", ""); err == nil && val != "" {
		var raw []int
		if err := json.Unmarshal([]byte(strings.TrimSpace(val)), &raw); err == nil && len(raw) > 0 {
			backoff := make([]time.Duration, 0, len(raw))
			for _, ms := range raw {
				if ms <= 0 {
					continue
				}
				backoff = append(backoff, time.Duration(ms)*time.Millisecond)
			}
			if len(backoff) > 0 {
				cfg.Delivery.RetryBackoff = backoff
			}
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/delivery/max_retries", ""); err == nil && val != "" {
		if retries, err := strconv.Atoi(val); err == nil {
			cfg.Delivery.MaxRetries = retries
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/delivery/ledger_path", ""); err == nil && val != "" {
		cfg.Delivery.LedgerPath = strings.TrimSpace(val)
	}

	deliveryHeartbeatInterval := time.Second
	if val, err := etcdClient.GetVarWithDefault(ctx, "core/delivery/heartbeat_interval_ms", ""); err == nil && val != "" {
		if ms, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && ms > 0 {
			deliveryHeartbeatInterval = time.Duration(ms) * time.Millisecond
		}
	}

	// Cargar gRPC KeepAlive cliente
	if val, err := etcdClient.GetVarWithDefault(ctx, "grpc/client_keepalive/time_s", ""); err == nil && val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			cfg.KeepAliveTime = time.Duration(seconds) * time.Second
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "grpc/client_keepalive/timeout_s", ""); err == nil && val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			cfg.KeepAliveTimeout = time.Duration(seconds) * time.Second
		}
	}
	if val, err := etcdClient.GetVarWithDefault(ctx, "grpc/client_keepalive/permit_without_stream", ""); err == nil && val != "" {
		if permit, err := strconv.ParseBool(val); err == nil {
			cfg.PermitWithoutStream = permit
		}
	}

	cfg.applyClientHeartbeatInterval(deliveryHeartbeatInterval)

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

	// Cargar Log Level desde agent/log_level
	if val, err := etcdClient.GetVarWithDefault(ctx, "agent/log_level", ""); err == nil && val != "" {
		cfg.LogLevel = val
	}

	// Validar configuración mínima requerida
	if cfg.CoreAddress == "" {
		return nil, fmt.Errorf("endpoints/core_addr not configured in ETCD")
	}
	if len(cfg.MasterAccounts) == 0 && len(cfg.SlaveAccounts) == 0 {
		return nil, fmt.Errorf("agent/master_accounts or agent/slave_accounts not configured in ETCD")
	}

	if err := cfg.validateKeepAlive(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultLedgerPath(agentID string) string {
	base := os.Getenv("PROGRAMDATA")
	if runtime.GOOS != "windows" {
		base = "/var/lib/echo"
	}
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "Echo", "agent", "acks", fmt.Sprintf("%s.db", agentID))
}

func (c *Config) validateKeepAlive() error {
	min := time.Duration(minClientHeartbeatSeconds) * time.Second
	max := time.Duration(maxClientHeartbeatSeconds) * time.Second

	if c.KeepAliveTime < min || c.KeepAliveTime > max {
		return fmt.Errorf("grpc/client_keepalive/time_s must be between %d and %d seconds, got %s",
			minClientHeartbeatSeconds, maxClientHeartbeatSeconds, c.KeepAliveTime)
	}
	if c.KeepAliveTimeout < min || c.KeepAliveTimeout > max {
		return fmt.Errorf("grpc/client_keepalive/timeout_s must be between %d and %d seconds, got %s",
			minClientHeartbeatSeconds, maxClientHeartbeatSeconds, c.KeepAliveTimeout)
	}
	if c.KeepAliveTimeout > c.KeepAliveTime {
		return fmt.Errorf("grpc/client_keepalive/timeout_s (%s) cannot exceed time_s (%s)",
			c.KeepAliveTimeout, c.KeepAliveTime)
	}
	return nil
}

func (c *Config) applyClientHeartbeatInterval(interval time.Duration) {
	min := time.Duration(minClientHeartbeatSeconds) * time.Second
	max := time.Duration(maxClientHeartbeatSeconds) * time.Second

	if interval <= 0 {
		interval = min
	}
	if interval < min {
		interval = min
	}
	if interval > max {
		interval = max
	}

	c.KeepAliveTime = interval

	timeout := interval / 2
	if timeout < min {
		timeout = min
	}
	if timeout > interval {
		timeout = interval
	}

	c.KeepAliveTimeout = timeout
}
