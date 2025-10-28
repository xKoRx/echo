package internal

import (
	"sync"
	"time"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// DedupeStore almacén de deduplicación in-memory.
//
// Mantiene un map de trade_id → DedupeEntry con TTL automático.
// Thread-safe para acceso concurrente.
type DedupeStore struct {
	entries map[string]*DedupeEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

// DedupeEntry entrada de deduplicación.
type DedupeEntry struct {
	TradeID   string
	Status    pb.OrderStatus
	Timestamp time.Time
	ClientID  string // Master que generó el intent
	Symbol    string
}

// NewDedupeStore crea un nuevo store de deduplicación.
func NewDedupeStore(ttl time.Duration) *DedupeStore {
	return &DedupeStore{
		entries: make(map[string]*DedupeEntry),
		ttl:     ttl,
	}
}

// Check verifica si un trade_id ya existe.
//
// Retorna:
//   - exists: true si el trade_id ya fue procesado
//   - entry: la entrada existente (nil si no existe)
func (d *DedupeStore) Check(tradeID string) (exists bool, entry *DedupeEntry) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	entry, exists = d.entries[tradeID]
	return exists, entry
}

// Add agrega una nueva entrada de trade.
//
// Retorna error si el trade_id ya existe con status != PENDING.
func (d *DedupeStore) Add(tradeID, clientID, symbol string, status pb.OrderStatus) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Verificar si ya existe
	if existing, exists := d.entries[tradeID]; exists {
		// Si está PENDING, permitir (podría ser reintento legítimo en i0)
		if existing.Status != pb.OrderStatus_ORDER_STATUS_PENDING {
			return &DedupeError{
				TradeID:        tradeID,
				ExistingStatus: existing.Status,
				Message:        "trade already processed",
			}
		}
	}

	// Agregar nueva entrada
	d.entries[tradeID] = &DedupeEntry{
		TradeID:   tradeID,
		Status:    status,
		Timestamp: time.Now(),
		ClientID:  clientID,
		Symbol:    symbol,
	}

	return nil
}

// UpdateStatus actualiza el status de un trade existente.
//
// No-op si el trade no existe.
func (d *DedupeStore) UpdateStatus(tradeID string, newStatus pb.OrderStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, exists := d.entries[tradeID]; exists {
		entry.Status = newStatus
	}
}

// Get obtiene una entrada por trade_id.
//
// Retorna nil si no existe.
func (d *DedupeStore) Get(tradeID string) *DedupeEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.entries[tradeID]
}

// Cleanup elimina entries antiguas (TTL expirado).
//
// Retorna el número de entries eliminadas.
func (d *DedupeStore) Cleanup() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	removed := 0

	for tradeID, entry := range d.entries {
		// Eliminar si:
		// - TTL expirado Y status != PENDING (ya procesado)
		if now.Sub(entry.Timestamp) > d.ttl && entry.Status != pb.OrderStatus_ORDER_STATUS_PENDING {
			delete(d.entries, tradeID)
			removed++
		}
	}

	return removed
}

// Size retorna el número de entries actuales.
func (d *DedupeStore) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}

// Clear limpia todas las entries (para testing).
func (d *DedupeStore) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = make(map[string]*DedupeEntry)
}

// DedupeError error de deduplicación (trade duplicado).
type DedupeError struct {
	TradeID        string
	ExistingStatus pb.OrderStatus
	Message        string
}

// Error implementa la interfaz error.
func (e *DedupeError) Error() string {
	return e.Message
}

