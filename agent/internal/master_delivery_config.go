package internal

import "sync"

// MasterDeliveryConfig mantiene el snapshot de parámetros que el Agent debe
// propagar hacia los Master EAs (vector de backoff y max retries).
type MasterDeliveryConfig struct {
	mu         sync.RWMutex
	backoff    []uint32
	maxRetries uint32
}

func NewMasterDeliveryConfig(backoff []uint32, maxRetries uint32) *MasterDeliveryConfig {
	cfg := &MasterDeliveryConfig{}
	cfg.Replace(backoff, maxRetries)
	return cfg
}

// Replace sobrescribe completamente el snapshot actual.
func (c *MasterDeliveryConfig) Replace(backoff []uint32, maxRetries uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.backoff = cloneUint32Slice(backoff)
	c.maxRetries = maxRetries
}

// Update aplica nuevos valores cuando están presentes (len(backoff)>0 o maxRetries>0)
// y retorna true si hubo algún cambio.
func (c *MasterDeliveryConfig) Update(backoff []uint32, maxRetries uint32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := false
	if len(backoff) > 0 {
		c.backoff = cloneUint32Slice(backoff)
		changed = true
	}
	if maxRetries > 0 && c.maxRetries != maxRetries {
		c.maxRetries = maxRetries
		changed = true
	}
	return changed
}

// Snapshot devuelve una copia del estado actual.
func (c *MasterDeliveryConfig) Snapshot() ([]uint32, uint32) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneUint32Slice(c.backoff), c.maxRetries
}

func cloneUint32Slice(values []uint32) []uint32 {
	if len(values) == 0 {
		return nil
	}
	cp := make([]uint32, len(values))
	copy(cp, values)
	return cp
}
