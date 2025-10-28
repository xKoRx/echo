// Package domain contiene tipos de dominio, validaciones y transformaciones para Echo.
//
// # Responsabilidades
//
// - Tipos enriquecidos (wrappers sobre mensajes proto)
// - Validaciones de negocio
// - Transformaciones Proto ↔ Domain ↔ JSON
// - Sistema de errores del dominio de trading
//
// # Tipos Enriquecidos
//
// Los tipos enriquecidos extienden los mensajes proto con validaciones y lógica de dominio:
//
//	intent := domain.NewTradeIntent(protoIntent)
//	if err := intent.Validate([]string{"XAUUSD"}); err != nil {
//	    // Intent inválido
//	}
//
// # Validaciones
//
// Validaciones de negocio para trading:
//
//	// Validar símbolo
//	err := domain.ValidateSymbol("XAUUSD", []string{"XAUUSD"})
//
//	// Validar lot size
//	err := domain.ValidateLotSize(0.10, 0.01, 100.0, 0.01)
//
//	// Validar mensaje completo
//	err := domain.ValidateTradeIntent(protoIntent, symbolWhitelist)
//
// # Transformadores
//
// Conversiones entre formatos:
//
// ## JSON → Proto
//
//	jsonMap, _ := utils.JSONToMap(jsonBytes)
//	intent, err := domain.JSONToTradeIntent(jsonMap)
//
// ## Proto → JSON
//
//	jsonMap, err := domain.TradeIntentToJSON(protoIntent)
//	jsonBytes, _ := utils.MapToJSON(jsonMap)
//
// ## Proto → Proto (con transformaciones)
//
//	opts := &domain.TransformOptions{
//	    LotSize:   0.10,
//	    CommandID: utils.GenerateUUIDv7(),
//	}
//	order := domain.TradeIntentToExecuteOrder(intent, opts)
//
// # Sistema de Errores
//
// Errores tipados con contexto:
//
//	err := domain.NewError(domain.ErrInvalidSymbol, "EURUSD not allowed")
//	err.WithDetail("symbol", "EURUSD")
//	err.WithDetail("whitelist", []string{"XAUUSD"})
//
//	// Wrapping
//	err := domain.WrapError(domain.ErrConnectionLost, "gRPC failed", originalErr)
//
//	// Conversión desde códigos MT4
//	code := domain.ErrorFromMT4Code(134) // => ErrNoMoney
//
// # Flujo Típico (Agent)
//
// Agent recibe JSON desde pipe, valida y transforma a proto para Core:
//
//	// 1. Leer JSON del pipe
//	jsonMap, err := utils.JSONToMap(jsonBytes)
//
//	// 2. Parsear a proto
//	intent, err := domain.JSONToTradeIntent(jsonMap)
//
//	// 3. Validar
//	if err := domain.ValidateTradeIntent(intent, symbolWhitelist); err != nil {
//	    // Rechazar
//	    return err
//	}
//
//	// 4. Enviar al Core vía gRPC
//	stream.Send(&pb.AgentMessage{
//	    Payload: &pb.AgentMessage_TradeIntent{TradeIntent: intent},
//	})
//
// # Flujo Típico (Core)
//
// Core recibe proto, transforma, y envía al Agent:
//
//	// 1. Recibir TradeIntent
//	intent := msg.GetTradeIntent()
//
//	// 2. Validar
//	if err := domain.ValidateTradeIntent(intent, symbolWhitelist); err != nil {
//	    return err
//	}
//
//	// 3. Calcular sizing (Money Management)
//	lotSize := calculateLotSize(intent)
//
//	// 4. Transformar
//	opts := &domain.TransformOptions{
//	    LotSize:   lotSize,
//	    CommandID: utils.GenerateUUIDv7(),
//	    ClientID:  "slave_67890",
//	}
//	order := domain.TradeIntentToExecuteOrder(intent, opts)
//
//	// 5. Enviar ExecuteOrder
//	stream.Send(&pb.CoreMessage{
//	    Payload: &pb.CoreMessage_ExecuteOrder{ExecuteOrder: order},
//	})
//
// # Integración con Otros Paquetes
//
// - sdk/utils: Parsing JSON, UUIDs, timestamps
// - sdk/telemetry: Logs estructurados con error codes
// - sdk/ipc: Transformaciones JSON para Named Pipes
// - Agent: Routing y validación
// - Core: Orquestación y Money Management
//
// # Principios de Diseño
//
//  1. Validación temprana: Rechazar datos inválidos en el edge (Agent)
//  2. Transformaciones explícitas: Sin conversiones implícitas
//  3. Inmutabilidad: Los tipos proto son read-only
//  4. Error context: Siempre agregar contexto a los errores
//  5. SDK-first: Toda lógica reutilizable aquí, no en Agent/Core
package domain

