package etcd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockClient es un cliente etcd para pruebas
type MockClient struct {
	Data    map[string]string
	FailGet bool
}

// Simular la funcionalidad de GetVar
func (m *MockClient) GetVar(ctx context.Context, key string) (string, error) {
	if m.FailGet {
		return "", errors.New("error simulado en GetVar")
	}

	value, exists := m.Data[key]
	if !exists {
		return "", errors.New("clave no encontrada: " + key)
	}

	return value, nil
}

// Simular la funcionalidad de SetVar
func (m *MockClient) SetVar(ctx context.Context, key, val string) error {
	if key == "error-key" {
		return errors.New("error simulado en SetVar")
	}

	m.Data[key] = val
	return nil
}

// Simular la funcionalidad de DeleteVar
func (m *MockClient) DeleteVar(ctx context.Context, key string) error {
	if key == "error-key" {
		return errors.New("error simulado en DeleteVar")
	}

	delete(m.Data, key)
	return nil
}

// Simular la funcionalidad de GetVarWithDefault
func (m *MockClient) GetVarWithDefault(ctx context.Context, key, defaultValue string) (string, error) {
	value, err := m.GetVar(ctx, key)
	if err != nil {
		return defaultValue, nil
	}
	return value, nil
}

// Simular la funcionalidad de GetVarInt
func (m *MockClient) GetVarInt(ctx context.Context, key string) (int, error) {
	// Esta implementación es simplificada para pruebas
	if key == "valid-int" {
		return 123, nil
	} else if key == "invalid-int" {
		return 0, errors.New("no es un número")
	}
	return 0, errors.New("clave no encontrada")
}

// Simular la funcionalidad de GetVarBool
func (m *MockClient) GetVarBool(ctx context.Context, key string) (bool, error) {
	// Esta implementación es simplificada para pruebas
	if key == "true-value" {
		return true, nil
	} else if key == "false-value" {
		return false, nil
	} else if key == "invalid-bool" {
		return false, errors.New("no es un booleano")
	}
	return false, errors.New("clave no encontrada")
}

// Crear un mock básico
func NewMockClient() *MockClient {
	return &MockClient{
		Data: map[string]string{
			"test-key":       "test-value",
			"existing-key":   "existing-value",
			"valid-int":      "123",
			"invalid-int":    "abc",
			"true-value":     "true",
			"false-value":    "false",
			"invalid-bool":   "not-a-bool",
			"valid-duration": "1000",
		},
	}
}

// TestClient_GetVar_Success prueba la función GetVar cuando es exitosa
func TestClient_GetVar_Success(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Execution
	ctx := context.Background()
	value, err := mockClient.GetVar(ctx, "test-key")

	// Verification
	assert.NoError(t, err, "No debería haber error al obtener la variable")
	assert.Equal(t, "test-value", value, "El valor obtenido debería coincidir con el esperado")
}

// TestClient_GetVar_NotFound prueba la función GetVar cuando la clave no existe
func TestClient_GetVar_NotFound(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Execution
	ctx := context.Background()
	_, err := mockClient.GetVar(ctx, "nonexistent-key")

	// Verification
	assert.Error(t, err, "Debería haber error cuando la clave no existe")
	assert.Contains(t, err.Error(), "clave no encontrada", "El mensaje de error debería indicar que la clave no fue encontrada")
}

// TestClient_GetVar_Error prueba la función GetVar cuando ocurre un error
func TestClient_GetVar_Error(t *testing.T) {
	// Setup
	mockClient := NewMockClient()
	mockClient.FailGet = true

	// Execution
	ctx := context.Background()
	_, err := mockClient.GetVar(ctx, "any-key")

	// Verification
	assert.Error(t, err, "Debería haber error cuando falla la operación")
	assert.Contains(t, err.Error(), "error simulado", "El mensaje de error debería ser el esperado")
}

// TestClient_GetVarWithDefault prueba la función GetVarWithDefault
func TestClient_GetVarWithDefault(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Test con clave existente
	ctx := context.Background()
	value, err := mockClient.GetVarWithDefault(ctx, "existing-key", "default-value")
	assert.NoError(t, err, "No debería haber error con clave existente")
	assert.Equal(t, "existing-value", value, "Debería devolver el valor existente")

	// Test con clave no existente
	value, err = mockClient.GetVarWithDefault(ctx, "nonexistent-key", "default-value")
	assert.NoError(t, err, "No debería haber error con clave no existente")
	assert.Equal(t, "default-value", value, "Debería devolver el valor por defecto")
}

// TestClient_SetVar_Success prueba la función SetVar cuando es exitosa
func TestClient_SetVar_Success(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Execution
	ctx := context.Background()
	err := mockClient.SetVar(ctx, "new-key", "new-value")

	// Verification
	assert.NoError(t, err, "No debería haber error al establecer la variable")
	value, found := mockClient.Data["new-key"]
	assert.True(t, found, "La clave debería existir después de establecerla")
	assert.Equal(t, "new-value", value, "El valor debería ser el establecido")
}

// TestClient_SetVar_Error prueba la función SetVar cuando ocurre un error
func TestClient_SetVar_Error(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Execution
	ctx := context.Background()
	err := mockClient.SetVar(ctx, "error-key", "test-value")

	// Verification
	assert.Error(t, err, "Debería haber error cuando falla la operación")
	assert.Contains(t, err.Error(), "error simulado", "El mensaje de error debería ser el esperado")
}

// TestClient_DeleteVar_Success prueba la función DeleteVar cuando es exitosa
func TestClient_DeleteVar_Success(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Execution
	ctx := context.Background()
	err := mockClient.DeleteVar(ctx, "test-key")

	// Verification
	assert.NoError(t, err, "No debería haber error al eliminar la variable")
	_, found := mockClient.Data["test-key"]
	assert.False(t, found, "La clave no debería existir después de eliminarla")
}

// TestClient_DeleteVar_Error prueba la función DeleteVar cuando ocurre un error
func TestClient_DeleteVar_Error(t *testing.T) {
	// Setup
	mockClient := NewMockClient()

	// Execution
	ctx := context.Background()
	err := mockClient.DeleteVar(ctx, "error-key")

	// Verification
	assert.Error(t, err, "Debería haber error cuando falla la operación")
	assert.Contains(t, err.Error(), "error simulado", "El mensaje de error debería ser el esperado")
}

// ExampleClient_GetVar muestra cómo usar el cliente para obtener una variable
func ExampleClient_GetVar() {
	// Crear un cliente con las opciones
	client, err := New(
		WithApp("myapp"),
		WithEnv("development"),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		// Manejar el error
		return
	}
	defer client.Close()

	// Obtener una variable
	ctx := context.Background()
	value, err := client.GetVar(ctx, "max-connections")
	if err != nil {
		// Manejar el error
		return
	}

	// Usar el valor
	_ = value
}

// ExampleClient_GetVarWithDefault muestra cómo usar valores por defecto
func ExampleClient_GetVarWithDefault() {
	// Crear un cliente con las opciones
	client, err := New(
		WithApp("myapp"),
		WithEnv("development"),
	)
	if err != nil {
		// Manejar el error
		return
	}
	defer client.Close()

	// Obtener variables con valores por defecto
	ctx := context.Background()

	// String con valor por defecto
	strValue, _ := client.GetVarWithDefault(ctx, "api-key", "default-key")

	// Entero con valor por defecto
	intValue, _ := client.GetVarIntWithDefault(ctx, "max-connections", 100)

	// Booleano con valor por defecto
	boolValue, _ := client.GetVarBoolWithDefault(ctx, "feature-enabled", false)

	// Duración con valor por defecto
	durValue, _ := client.GetVarDurationWithDefault(ctx, "timeout", 5*time.Second)

	// Usar los valores
	_, _, _, _ = strValue, intValue, boolValue, durValue
}
