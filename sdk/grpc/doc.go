// Package grpc provee abstracciones de alto nivel para comunicación gRPC.
//
// Este paquete simplifica el uso de gRPC para comunicación Agent ↔ Core,
// incluyendo cliente/servidor con reconnection, streaming bidireccional,
// e interceptors de telemetría.
//
// # Cliente gRPC
//
// Crear un cliente con reconnection automática:
//
//	config := grpc.DefaultClientConfig("192.168.31.60:50051")
//	client, err := grpc.NewClient(ctx, config)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	// Usar con servicio generado
//	serviceClient := pb.NewAgentServiceClient(client.Conn())
//
// # Servidor gRPC
//
// Crear un servidor con interceptors:
//
//	config := grpc.DefaultServerConfig(50051)
//	server, err := grpc.NewServer(config)
//	if err != nil {
//	    return err
//	}
//
//	// Registrar servicios
//	pb.RegisterAgentServiceServer(server.GRPCServer(), &myService{})
//
//	// Servir
//	if err := server.Serve(ctx); err != nil {
//	    return err
//	}
//
// # Streaming Bidireccional
//
// Usar helpers de streams para simplificar send/recv:
//
//	// Cliente
//	rawStream, err := serviceClient.StreamBidi(ctx)
//	stream := grpc.NewStreamClient(rawStream)
//	defer stream.Close()
//
//	// Enviar (thread-safe)
//	stream.Send(&pb.AgentMessage{...})
//
//	// Recibir
//	for msg := range stream.Receive() {
//	    // Procesar mensaje
//	}
//
//	// Servidor
//	func (s *myService) StreamBidi(rawStream pb.AgentService_StreamBidiServer) error {
//	    stream := grpc.NewStreamServer(rawStream)
//	    defer stream.Close()
//
//	    for msg := range stream.Receive() {
//	        // Procesar y responder
//	        stream.Send(response)
//	    }
//
//	    return nil
//	}
//
// # Interceptors
//
// Agregar telemetría y tracing:
//
//	// Cliente
//	config := grpc.DefaultClientConfig("192.168.31.60:50051")
//	config.UnaryInterceptors = []grpc.UnaryClientInterceptor{
//	    grpc.LoggingUnaryClientInterceptor(telemetryClient),
//	    grpc.TracingUnaryClientInterceptor(),
//	}
//	config.StreamInterceptors = []grpc.StreamClientInterceptor{
//	    grpc.LoggingStreamClientInterceptor(telemetryClient),
//	    grpc.TracingStreamClientInterceptor(),
//	}
//
//	// Servidor
//	config := grpc.DefaultServerConfig(50051)
//	config.UnaryInterceptors = []grpc.UnaryServerInterceptor{
//	    grpc.LoggingUnaryServerInterceptor(telemetryClient),
//	    grpc.TracingUnaryServerInterceptor(),
//	}
//	config.StreamInterceptors = []grpc.StreamServerInterceptor{
//	    grpc.LoggingStreamServerInterceptor(telemetryClient),
//	    grpc.TracingStreamServerInterceptor(),
//	}
//
// # Trace ID Propagation
//
// Propagar trace_id entre Agent y Core:
//
//	// En Master EA (inicio)
//	tradeID := utils.GenerateUUIDv7()
//	ctx = grpc.SetTraceID(ctx, tradeID)
//
//	// En Agent (forwarding)
//	stream.Send(&pb.AgentMessage{...}) // trace_id se propaga automáticamente
//
//	// En Core (recepción)
//	traceID := grpc.GetTraceID(ctx) // Extrae trace_id del metadata
//
// # Reconnection
//
// El cliente maneja reconnection automática:
//
//	// Ejecutar con retry
//	err := client.WithRetry(ctx, func() error {
//	    _, err := serviceClient.SomeMethod(ctx, req)
//	    return err
//	})
//
//	// O reconectar manualmente
//	if !client.IsConnected() {
//	    if err := client.Reconnect(ctx); err != nil {
//	        return err
//	    }
//	}
//
// # Graceful Shutdown
//
// El servidor maneja graceful shutdown:
//
//	// Shutdown con timeout
//	if err := server.Shutdown(ctx); err != nil {
//	    log.Errorf("Shutdown error: %v", err)
//	}
//
//	// O forzar stop inmediato
//	server.Stop()
//
// # KeepAlive
//
// Configurar keepalive para detectar conexiones perdidas:
//
//	// Cliente
//	config.KeepAlive = &grpc.KeepAliveConfig{
//	    Time:                30 * time.Second,
//	    Timeout:             10 * time.Second,
//	    PermitWithoutStream: true,
//	}
//
//	// Servidor
//	config.KeepAlive = &grpc.ServerKeepAliveConfig{
//	    MaxConnectionIdle:     5 * time.Minute,
//	    MaxConnectionAge:      0, // Sin límite
//	    MaxConnectionAgeGrace: 1 * time.Minute,
//	    Time:                  2 * time.Hour,
//	    Timeout:               20 * time.Second,
//	}
//
// # Integración con Echo
//
// Este paquete es usado por:
//   - Agent: Cliente gRPC al Core
//   - Core: Servidor gRPC para Agents
//   - Ambos: Streaming bidireccional para TradeIntents/ExecuteOrders
//
// Flujo típico:
//
//  1. Agent crea cliente y conecta al Core
//  2. Agent abre stream bidireccional
//  3. Agent lee de Named Pipes (Master EA)
//  4. Agent transforma JSON → Proto y envía al Core
//  5. Core procesa y responde con ExecuteOrder
//  6. Agent transforma Proto → JSON y escribe a Named Pipes (Slave EA)
//
// # Referencias
//
// - google.golang.org/grpc: Implementación gRPC
// - sdk/telemetry: Integración de observabilidad
// - RFC-002: Especificación de comunicación gRPC
package grpc
