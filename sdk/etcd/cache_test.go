package etcd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockCache simula una implementación de CacheClient para pruebas
type MockCache struct {
	data map[string]string
}

// NewMockCache crea una nueva instancia de MockCache
func NewMockCache() *MockCache {
	return &MockCache{
		data: map[string]string{
			"test-key": "test-value",
		},
	}
}

// Get implementa la interfaz CacheClient
func (m *MockCache) Get(key string) (string, bool) {
	val, ok := m.data[key]
	return val, ok
}

// Close implementa la interfaz CacheClient
func (m *MockCache) Close() error {
	return nil
}

// Reload implementa la interfaz CacheClient
func (m *MockCache) Reload() error {
	// Simular una recarga añadiendo una nueva clave
	m.data["reloaded-key"] = "reloaded-value"
	return nil
}

// TestCache_Get prueba las operaciones básicas de la caché
func TestCache_Get(t *testing.T) {
	// Setup
	cache := NewMockCache()

	// Test valor existente
	value, found := cache.Get("test-key")
	assert.True(t, found, "Debería encontrar la clave existente")
	assert.Equal(t, "test-value", value, "El valor debería coincidir")

	// Test valor inexistente
	_, found = cache.Get("nonexistent-key")
	assert.False(t, found, "No debería encontrar una clave inexistente")

	// Test recarga
	err := cache.Reload()
	assert.NoError(t, err, "Reload no debería dar error")

	// Verificar que la recarga funcionó
	value, found = cache.Get("reloaded-key")
	assert.True(t, found, "Debería encontrar la clave añadida en la recarga")
	assert.Equal(t, "reloaded-value", value, "El valor recargado debería coincidir")
}

// ExampleCache muestra cómo usar la caché
func ExampleCache() {
	// Crear un cliente etcd
	client, err := New(
		WithApp("myapp"),
		WithEnv("development"),
	)
	if err != nil {
		// Manejar error
		return
	}

	// Crear una caché
	cache, err := NewCache(client)
	if err != nil {
		// Manejar error
		return
	}
	defer cache.Close()

	// Obtener un valor de la caché
	value, ok := cache.Get("/config/database-url")
	if !ok {
		// Valor no encontrado en la caché
		return
	}

	// Usar el valor
	_ = value

	// Forzar recarga de la caché
	if err := cache.Reload(); err != nil {
		// Manejar error de recarga
		return
	}
}
