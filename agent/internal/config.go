// Package internal contiene configuración del Agent cargada desde ETCD.
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

// Config configuración del Agent (i1).
//
// Cargada desde ETCD en namespace echo/{environment}.
type Config struct {
	// Core
	CoreAddress string // endpoints/core_addr

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

	// Telemetry
	ServiceName     string // telemetry/service_name
	ServiceVersion  string // telemetry/service_version
	Environment     string // telemetry/environment
	OTLPEndpoint    string // endpoints/otel/otlp_endpoint
	MetricsEndpoint string // endpoints/otel/metrics_endpoint
	LogLevel        string // agent/log_level (INFO, DEBUG, WARN, ERROR)
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
		KeepAliveTime:       60 * time.Second,
		KeepAliveTimeout:    20 * time.Second,
		PermitWithoutStream: false,
		ServiceName:         "echo-agent",
		ServiceVersion:      "1.0.0-i1",
		Environment:         env,
		LogLevel:            "INFO", // Default
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

	return cfg, nil
}
