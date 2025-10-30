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

// Close no hace nada en plataformas no-Windows.
func (h *PipeHandler) Close() error {
	return nil
}
