// Package ipc provee abstracciones para comunicación inter-proceso via Named Pipes.
//
// Este paquete implementa Named Pipes de Windows para comunicación
// bidireccional entre el Agent (Go) y los Expert Advisors (MQL4/MQL5).
//
// # Arquitectura
//
// - Agent actúa como servidor (crea Named Pipes)
// - EAs actúan como clientes (se conectan via DLL echo_pipe.dll)
// - Protocolo: JSON line-delimited (cada mensaje termina con \n)
// - Buffering: Lecturas eficientes con bufio.Scanner
// - Thread-safety: Writes serializados con mutex
//
// # Uso Básico: Server (Agent)
//
// Crear un servidor de Named Pipe:
//
//	pipe, err := ipc.NewWindowsPipeServer("echo_master_12345")
//	if err != nil {
//	    return err
//	}
//	defer pipe.Close()
//
//	// Esperar conexión del EA
//	if err := pipe.WaitForConnection(ctx); err != nil {
//	    return err
//	}
//
//	// Leer mensajes
//	reader := ipc.NewJSONReader(pipe)
//	for {
//	    msg, err := reader.ReadMessage()
//	    if err != nil {
//	        break
//	    }
//	    // Procesar mensaje...
//	}
//
// # Uso Básico: Writer
//
// Escribir mensajes al pipe:
//
//	writer := ipc.NewJSONWriter(pipe)
//
//	msg := map[string]interface{}{
//	    "type": "execute_order",
//	    "timestamp_ms": utils.NowUnixMilli(),
//	    "payload": map[string]interface{}{
//	        "command_id": utils.GenerateUUIDv7(),
//	        "trade_id": "abc123",
//	        "symbol": "XAUUSD",
//	        "order_side": "BUY",
//	        "lot_size": 0.10,
//	        "magic_number": 123456,
//	    },
//	}
//
//	if err := writer.WriteMessage(msg); err != nil {
//	    return err
//	}
//
// # Protocolo JSON Line-Delimited
//
// Cada mensaje es un objeto JSON en una línea, terminado con \n:
//
//	{"type":"trade_intent","timestamp_ms":1698345601000,"payload":{...}}\n
//	{"type":"execute_order","timestamp_ms":1698345601050,"payload":{...}}\n
//
// ## Ejemplo de TradeIntent (Master EA → Agent)
//
//	{
//	  "type": "trade_intent",
//	  "timestamp_ms": 1698345601000,
//	  "payload": {
//	    "trade_id": "01HKQV8Y-9GJ3-F5R6-WN8P-2M4D1E",
//	    "client_id": "master_12345",
//	    "symbol": "XAUUSD",
//	    "order_side": "BUY",
//	    "lot_size": 0.01,
//	    "price": 2045.50,
//	    "magic_number": 123456,
//	    "ticket": 987654
//	  }
//	}
//
// ## Ejemplo de ExecuteOrder (Agent → Slave EA)
//
//	{
//	  "type": "execute_order",
//	  "timestamp_ms": 1698345601050,
//	  "payload": {
//	    "command_id": "01HKQV8Y-ABCD-EF12-3456-789XYZ",
//	    "trade_id": "01HKQV8Y-9GJ3-F5R6-WN8P-2M4D1E",
//	    "symbol": "XAUUSD",
//	    "order_side": "BUY",
//	    "lot_size": 0.10,
//	    "magic_number": 123456
//	  }
//	}
//
// # Características
//
// ## Buffering
//
// - JSONReader usa bufio.Scanner con buffer de 1MB
// - Lecturas eficientes para mensajes grandes
// - Timeout configurable por operación
//
// ## Thread Safety
//
// - JSONWriter serializa writes con sync.Mutex
// - Múltiples goroutines pueden escribir al mismo pipe sin race conditions
// - Lecturas deben hacerse desde una sola goroutine
//
// ## Error Handling
//
// - ErrPipeClosed: Pipe fue cerrado
// - ErrTimeout: Operación excedió timeout
// - ErrInvalidMessage: JSON inválido o mensaje malformado
//
// # Timeouts
//
// Configurar timeout para operaciones:
//
//	reader := ipc.NewJSONReaderWithTimeout(pipe, 10*time.Second)
//	writer := ipc.NewJSONWriterWithTimeout(pipe, 5*time.Second)
//
//	// O cambiar dinámicamente
//	reader.SetTimeout(2 * time.Second)
//	writer.SetTimeout(3 * time.Second)
//
// # Bidireccional
//
// Para comunicación full-duplex:
//
//	rw := ipc.NewBufferedWriter(pipe)
//
//	// Escribir
//	rw.WriteMessage(msg)
//
//	// Leer
//	response, err := rw.ReadMessage()
//
// # Integración con Agent
//
// El Agent usa este paquete para:
//
//  1. Crear pipes por cada EA (master/slave)
//  2. Leer TradeIntents de Master EAs
//  3. Enviar ExecuteOrders a Slave EAs
//  4. Recibir ExecutionResults de Slaves
//
// Flujo típico en Agent:
//
//	// Crear pipe para master
//	masterPipe, _ := ipc.NewWindowsPipeServer("echo_master_12345")
//	masterPipe.WaitForConnection(ctx)
//
//	// Leer intents
//	reader := ipc.NewJSONReader(masterPipe)
//	go func() {
//	    for {
//	        msg, err := reader.ReadMessage()
//	        if err != nil { break }
//	        // Transformar y enviar al Core via gRPC
//	        intent, _ := domain.JSONToTradeIntent(msg)
//	        grpcStream.Send(&pb.AgentMessage{
//	            Payload: &pb.AgentMessage_TradeIntent{TradeIntent: intent},
//	        })
//	    }
//	}()
//
//	// Crear pipe para slave
//	slavePipe, _ := ipc.NewWindowsPipeServer("echo_slave_67890")
//	slavePipe.WaitForConnection(ctx)
//
//	// Escribir órdenes
//	writer := ipc.NewJSONWriter(slavePipe)
//	go func() {
//	    for order := range orderChannel {
//	        jsonMsg, _ := domain.ExecuteOrderToJSON(order)
//	        writer.WriteMessage(jsonMsg)
//	    }
//	}()
//
// # Plataforma
//
// Este paquete está diseñado específicamente para Windows.
// El build tag `// +build windows` asegura que solo se compile en Windows.
//
// Para soporte futuro de otras plataformas (Linux/Mac), se puede
// implementar un wrapper sobre Unix Domain Sockets con la misma interfaz.
//
// # Referencias
//
// - github.com/Microsoft/go-winio: Implementación de Named Pipes
// - sdk/domain: Transformadores JSON ↔ Proto
// - sdk/utils: Helpers de JSON
// - RFC-002: Especificación del protocolo IPC
package ipc

