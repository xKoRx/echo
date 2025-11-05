package internal

import (
	"sync"

	"github.com/xKoRx/echo/sdk/domain/handshake"
)

// HandshakeRegistry almacena la última evaluación por cuenta.
type HandshakeRegistry struct {
	mu          sync.RWMutex
	evaluations map[string]*handshake.Evaluation
}

// NewHandshakeRegistry crea un registro vacío.
func NewHandshakeRegistry() *HandshakeRegistry {
	return &HandshakeRegistry{
		evaluations: make(map[string]*handshake.Evaluation),
	}
}

// Set actualiza la evaluación asociada a la cuenta.
func (r *HandshakeRegistry) Set(evaluation *handshake.Evaluation) {
	if evaluation == nil {
		return
	}
	r.mu.Lock()
	r.evaluations[evaluation.AccountID] = evaluation
	r.mu.Unlock()
}

// Get retorna la evaluación almacenada para una cuenta.
func (r *HandshakeRegistry) Get(accountID string) (*handshake.Evaluation, bool) {
	r.mu.RLock()
	evaluation, ok := r.evaluations[accountID]
	r.mu.RUnlock()
	return evaluation, ok
}

// Status entrega el estado actual de registro para la cuenta.
func (r *HandshakeRegistry) Status(accountID string) handshake.RegistrationStatus {
	r.mu.RLock()
	evaluation, ok := r.evaluations[accountID]
	r.mu.RUnlock()
	if !ok || evaluation == nil {
		return handshake.RegistrationStatusUnspecified
	}
	return evaluation.Status
}

// Clear elimina una cuenta del registro.
func (r *HandshakeRegistry) Clear(accountID string) {
	r.mu.Lock()
	delete(r.evaluations, accountID)
	r.mu.Unlock()
}

// Accounts retorna la lista de cuentas registradas.
func (r *HandshakeRegistry) Accounts() []string {
	r.mu.RLock()
	ids := make([]string, 0, len(r.evaluations))
	for id := range r.evaluations {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	return ids
}
