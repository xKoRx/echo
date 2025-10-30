package internal

import (
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

// AccountRegistry mantiene el mapeo de cuentas a Agents (estado OPERACIONAL).
//
// Thread-safe. Operaciones:
//   - RegisterAccount: registra UNA cuenta para un Agent (i2 dinámico).
//   - UnregisterAccount: desregistra UNA cuenta (i2 dinámico).
//   - UnregisterAgent: elimina TODAS las cuentas de un Agent (cleanup al desconectar).
//   - GetOwner: retorna el Agent propietario de una cuenta.
//   - GetAccountsByAgent: retorna todas las cuentas de un Agent (diagnóstico).
type AccountRegistry struct {
	// account_id → OwnershipRecord
	accountToOwner map[string]*OwnershipRecord
	// agent_id → []account_id (índice inverso para cleanup)
	agentToAccounts map[string][]string

	mu        sync.RWMutex
	telemetry *telemetry.Client
}

// OwnershipRecord registra ownership de una cuenta (i2).
type OwnershipRecord struct {
	AgentID      string
	AccountID    string
	RegisteredAt time.Time
	LastSeenAt   time.Time // actualizado en cada heartbeat (opcional i3+)
}

// NewAccountRegistry crea un nuevo registry.
func NewAccountRegistry(tel *telemetry.Client) *AccountRegistry {
	return &AccountRegistry{
		accountToOwner:  make(map[string]*OwnershipRecord),
		agentToAccounts: make(map[string][]string),
		telemetry:       tel,
	}
}

// RegisterAccount registra UNA cuenta para un Agent (i2 dinámico).
//
// Si la cuenta ya está registrada a OTRO Agent, sobreescribe (last-write-wins).
// Log WARNING si hay cambio de ownership.
func (r *AccountRegistry) RegisterAccount(agentID string, accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Verificar si ya existe
	if existing, exists := r.accountToOwner[accountID]; exists {
		if existing.AgentID != agentID {
			// Conflicto de ownership: la cuenta estaba en otro Agent
			r.telemetry.Warn(nil, "Account ownership conflict (i2)",
				attribute.String("account_id", accountID),
				attribute.String("previous_agent", existing.AgentID),
				attribute.String("new_agent", agentID),
			)

			// Limpiar del agente anterior
			r.removeAccountFromAgent(existing.AgentID, accountID)
		} else {
			// Mismo agente, actualizar timestamp (re-registro, posible reconexión)
			existing.LastSeenAt = now
			r.telemetry.Info(nil, "Account re-registered to same Agent (i2)",
				attribute.String("account_id", accountID),
				attribute.String("agent_id", agentID),
			)
			return
		}
	}

	// Registrar nueva cuenta
	r.accountToOwner[accountID] = &OwnershipRecord{
		AgentID:      agentID,
		AccountID:    accountID,
		RegisteredAt: now,
		LastSeenAt:   now,
	}

	// Añadir a índice inverso
	r.agentToAccounts[agentID] = append(r.agentToAccounts[agentID], accountID)

	r.telemetry.Info(nil, "Account registered to Agent (i2)",
		attribute.String("agent_id", agentID),
		attribute.String("account_id", accountID),
	)
}

// UnregisterAccount desregistra UNA cuenta (i2 dinámico).
//
// Se llama cuando el Slave EA se desconecta del Agent.
func (r *AccountRegistry) UnregisterAccount(accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, exists := r.accountToOwner[accountID]
	if !exists {
		r.telemetry.Warn(nil, "Attempted to unregister non-existent account (i2)",
			attribute.String("account_id", accountID),
		)
		return
	}

	agentID := record.AgentID

	// Eliminar del mapa principal
	delete(r.accountToOwner, accountID)

	// Eliminar del índice inverso
	r.removeAccountFromAgent(agentID, accountID)

	r.telemetry.Info(nil, "Account unregistered from Agent (i2)",
		attribute.String("agent_id", agentID),
		attribute.String("account_id", accountID),
	)
}

// UnregisterAgent elimina TODAS las cuentas de un Agent (i2 cleanup).
//
// Se llama al desconectar el Agent del Core.
func (r *AccountRegistry) UnregisterAgent(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	accounts, exists := r.agentToAccounts[agentID]
	if !exists {
		return
	}

	for _, acc := range accounts {
		delete(r.accountToOwner, acc)
	}
	delete(r.agentToAccounts, agentID)

	r.telemetry.Info(nil, "Agent unregistered, all accounts released (i2)",
		attribute.String("agent_id", agentID),
		attribute.Int("accounts_count", len(accounts)),
	)
}

// GetOwner retorna el Agent propietario de una cuenta.
//
// Retorna ("", false) si la cuenta no está registrada (o se desconectó).
func (r *AccountRegistry) GetOwner(accountID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, found := r.accountToOwner[accountID]
	if !found {
		return "", false
	}
	return record.AgentID, true
}

// GetAccountsByAgent retorna todas las cuentas de un Agent (diagnóstico).
func (r *AccountRegistry) GetAccountsByAgent(agentID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	accounts := r.agentToAccounts[agentID]
	// Retornar copia para evitar modificaciones externas
	result := make([]string, len(accounts))
	copy(result, accounts)
	return result
}

// GetStats retorna estadísticas del registry (diagnóstico/métricas).
func (r *AccountRegistry) GetStats() (totalAccounts int, totalAgents int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.accountToOwner), len(r.agentToAccounts)
}

// removeAccountFromAgent elimina una cuenta del índice inverso (helper interno).
//
// DEBE llamarse con lock ya adquirido.
func (r *AccountRegistry) removeAccountFromAgent(agentID string, accountID string) {
	accounts := r.agentToAccounts[agentID]
	for i, acc := range accounts {
		if acc == accountID {
			// Eliminar usando swap-and-truncate (eficiente)
			accounts[i] = accounts[len(accounts)-1]
			r.agentToAccounts[agentID] = accounts[:len(accounts)-1]
			break
		}
	}

	// Si el Agent ya no tiene cuentas, eliminar entrada
	if len(r.agentToAccounts[agentID]) == 0 {
		delete(r.agentToAccounts, agentID)
	}
}

