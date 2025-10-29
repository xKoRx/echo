package etcd

import (
	"net/http"
)

// TestEtcdMiddleware_ClientInjection verifica que el cliente sea inyectado correctamente en el contexto
/*func TestEtcdMiddleware_ClientInjection(t *testing.T) {
	// Setup
	// Usamos un cliente real ya que el middleware solo acepta *Client
	mockClient := &Client{
		app:     "testapp",
		env:     "test",
		timeout: 5,
	}

	// Crear middleware
	middleware := EtcdMiddleware(mockClient)

	// Crear handler final que verificará si el cliente está en el contexto
	var clientFromContext *Client
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientFromContext = GetEtcdClient(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Aplicar middleware al handler
	handlerToTest := middleware(nextHandler)

	// Crear request de prueba
	req := httptest.NewRequest("GET", "/test", nil)
	res := httptest.NewRecorder()

	// Execution
	handlerToTest.ServeHTTP(res, req)

	// Verification
	assert.Equal(t, http.StatusOK, res.Code, "El código de estado debería ser 200 OK")
	assert.NotNil(t, clientFromContext, "El cliente debería estar presente en el contexto")
}

// TestEtcdMiddleware_NilClient verifica el comportamiento cuando se pasa un cliente nil
func TestEtcdMiddleware_NilClient(t *testing.T) {
	// Setup
	// Pasamos nil como cliente
	middleware := EtcdMiddleware(nil)

	// Crear handler final que no debería ser llamado
	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Aplicar middleware al handler
	handlerToTest := middleware(nextHandler)

	// Crear request de prueba
	req := httptest.NewRequest("GET", "/test", nil)
	res := httptest.NewRecorder()

	// Execution
	handlerToTest.ServeHTTP(res, req)

	// Verification
	assert.Equal(t, http.StatusInternalServerError, res.Code, "El código de estado debería ser 500 Internal Server Error")
	assert.False(t, nextHandlerCalled, "El siguiente handler no debería ser llamado cuando el cliente es nil")
}

// TestContext_ClientManagement prueba las funciones de gestión del cliente en el contexto
func TestContext_ClientManagement(t *testing.T) {
	// Setup
	mockClient := &Client{
		app: "testapp",
		env: "test",
	}
	ctx := SetEtcdClient(nil, mockClient)

	// Verificar que se puede recuperar el cliente
	retrievedClient := GetEtcdClient(ctx)

	// Verification
	assert.NotNil(t, retrievedClient, "El cliente recuperado no debería ser nil")
	assert.Equal(t, mockClient, retrievedClient, "El cliente recuperado debería ser el mismo que se insertó")

	// Verificar con un contexto sin cliente
	emptyClient := GetEtcdClient(nil)
	assert.Nil(t, emptyClient, "GetEtcdClient debería devolver nil para un contexto sin cliente")
}*/

// ExampleEtcdMiddleware muestra cómo usar el middleware
func ExampleEtcdMiddleware() {
	// Crear cliente etcd
	client, err := New(
		WithApp("myapp"),
		WithEnv("development"),
	)
	if err != nil {
		// Manejar error
		return
	}

	// Crear middleware
	middleware := EtcdMiddleware(client)

	// Aplicar middleware a un router/mux
	router := http.NewServeMux()
	// Ejemplo con handler que usa el cliente desde el contexto
	router.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		etcdClient := GetEtcdClient(r.Context())
		if etcdClient == nil {
			http.Error(w, "Cliente etcd no disponible", http.StatusInternalServerError)
			return
		}

		// Usar el cliente para obtener configuración
		ctx := r.Context()
		value, err := etcdClient.GetVar(ctx, "api-key")
		if err != nil {
			http.Error(w, "Error al obtener configuración", http.StatusInternalServerError)
			return
		}

		w.Write([]byte(value))
	})

	// Wrap router with middleware
	wrappedRouter := middleware(router)

	// Start server with wrapped router
	http.ListenAndServe(":8080", wrappedRouter)
}
