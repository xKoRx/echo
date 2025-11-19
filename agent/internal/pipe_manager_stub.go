//go:build !windows
// +build !windows

package internal

import (
	"context"
	"fmt"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
)

// PipeManager stub para plataformas no-Windows.
//
// Named Pipes solo están soportados en Windows.
// Este stub permite compilar el proyecto en otras plataformas para desarrollo.
type PipeManager struct{}

// NewPipeManager retorna error en plataformas no-Windows.
func NewPipeManager(ctx context.Context, config *Config, tel *telemetry.Client, metrics *metricbundle.EchoMetrics) (*PipeManager, error) {
	return nil, fmt.Errorf("Named Pipes are only supported on Windows")
}

// Start retorna error en plataformas no-Windows.
func (pm *PipeManager) Start(sendToCoreCh chan *pb.AgentMessage, agentID string) error {
	return fmt.Errorf("Named Pipes are only supported on Windows")
}

func (pm *PipeManager) SetHandshakeCallback(cb func(string))   {}
func (pm *PipeManager) SetDeliveryManager(dm *DeliveryManager) {}
func (pm *PipeManager) SetMasterDeliveryConfig(cfg *MasterDeliveryConfig) {
}
func (pm *PipeManager) BroadcastDeliveryConfig(backoff []uint32, maxRetries uint32) {
}

// GetPipe retorna nil en plataformas no-Windows.
func (pm *PipeManager) GetPipe(name string) (*PipeHandler, bool) {
	return nil, false
}

// Close no hace nada en plataformas no-Windows.
func (pm *PipeManager) Close() error {
	return nil
}

// PipeHandler stub para plataformas no-Windows.
type PipeHandler struct{}

// WriteMessage retorna error en plataformas no-Windows.
func (h *PipeHandler) WriteMessage(msg interface{}) error {
	return fmt.Errorf("Named Pipes are only supported on Windows")
}

func (h *PipeHandler) WriteSymbolRegistrationResult(result *pb.SymbolRegistrationResult) error {
	return fmt.Errorf("Named Pipes are only supported on Windows")
}

// Close no hace nada en plataformas no-Windows.
func (h *PipeHandler) Close() error {
	return nil
}
