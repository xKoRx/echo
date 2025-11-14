package domain

import (
	"fmt"
	"strconv"
	"strings"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/utils"
)

// parseTimestamps extrae TimestampMetadata de un map JSON.
func parseTimestamps(payload map[string]interface{}) *pb.TimestampMetadata {
	tsMap, ok := payload["timestamps"].(map[string]interface{})
	if !ok {
		return nil
	}

	return &pb.TimestampMetadata{
		T0MasterEaMs:    utils.ExtractInt64(tsMap, "t0_master_ea_ms"),
		T1AgentRecvMs:   utils.ExtractInt64(tsMap, "t1_agent_recv_ms"),
		T2CoreRecvMs:    utils.ExtractInt64(tsMap, "t2_core_recv_ms"),
		T3CoreSendMs:    utils.ExtractInt64(tsMap, "t3_core_send_ms"),
		T4AgentRecvMs:   utils.ExtractInt64(tsMap, "t4_agent_recv_ms"),
		T5SlaveEaRecvMs: utils.ExtractInt64(tsMap, "t5_slave_ea_recv_ms"),
		T6OrderSendMs:   utils.ExtractInt64(tsMap, "t6_order_send_ms"),
		T7OrderFilledMs: utils.ExtractInt64(tsMap, "t7_order_filled_ms"),
	}
}

// timestampsToMap convierte TimestampMetadata a map JSON.
func timestampsToMap(ts *pb.TimestampMetadata) map[string]interface{} {
	if ts == nil {
		return nil
	}

	return map[string]interface{}{
		"t0_master_ea_ms":     ts.T0MasterEaMs,
		"t1_agent_recv_ms":    ts.T1AgentRecvMs,
		"t2_core_recv_ms":     ts.T2CoreRecvMs,
		"t3_core_send_ms":     ts.T3CoreSendMs,
		"t4_agent_recv_ms":    ts.T4AgentRecvMs,
		"t5_slave_ea_recv_ms": ts.T5SlaveEaRecvMs,
		"t6_order_send_ms":    ts.T6OrderSendMs,
		"t7_order_filled_ms":  ts.T7OrderFilledMs,
	}
}

// cloneTimestamps crea una copia profunda de TimestampMetadata para evitar
// compartir estado entre múltiples mensajes (por ejemplo, múltiples ExecuteOrder
// derivados de un mismo TradeIntent).
func cloneTimestamps(ts *pb.TimestampMetadata) *pb.TimestampMetadata {
	if ts == nil {
		return nil
	}
	return &pb.TimestampMetadata{
		T0MasterEaMs:    ts.T0MasterEaMs,
		T1AgentRecvMs:   ts.T1AgentRecvMs,
		T2CoreRecvMs:    ts.T2CoreRecvMs,
		T3CoreSendMs:    ts.T3CoreSendMs,
		T4AgentRecvMs:   ts.T4AgentRecvMs,
		T5SlaveEaRecvMs: ts.T5SlaveEaRecvMs,
		T6OrderSendMs:   ts.T6OrderSendMs,
		T7OrderFilledMs: ts.T7OrderFilledMs,
	}
}

func adjustableStopsToJSON(stops *pb.AdjustableStops) map[string]interface{} {
	if stops == nil {
		return nil
	}
	result := map[string]interface{}{
		"sl_offset_points": stops.SlOffsetPoints,
		"tp_offset_points": stops.TpOffsetPoints,
		"stop_level_breach": stops.StopLevelBreach,
	}
	if stops.Reason != "" {
		result["reason"] = stops.Reason
	}
	return result
}

func adjustableStopsFromJSON(raw interface{}) *pb.AdjustableStops {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	stops := &pb.AdjustableStops{}
	if val, exists := m["sl_offset_points"]; exists {
		stops.SlOffsetPoints = int32(utils.ExtractInt64(map[string]interface{}{"value": val}, "value"))
	}
	if val, exists := m["tp_offset_points"]; exists {
		stops.TpOffsetPoints = int32(utils.ExtractInt64(map[string]interface{}{"value": val}, "value"))
	}
	if val, exists := m["stop_level_breach"]; exists {
		if boolVal, ok := val.(bool); ok {
			stops.StopLevelBreach = boolVal
		} else {
			stops.StopLevelBreach = utils.ExtractBool(map[string]interface{}{"value": val}, "value")
		}
	}
	if val, exists := m["reason"]; exists {
		if str, ok := val.(string); ok {
			stops.Reason = str
		}
	}
	return stops
}

// JSONToTradeIntent convierte un map JSON a TradeIntent proto.
//
// Formato JSON esperado (desde Master EA):
//
//	{
//	  "type": "trade_intent",
//	  "timestamp_ms": 1698345601000,
//	  "payload": {
//	    "trade_id": "01HKQV8Y-9GJ3-...",
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
// Example:
//
//	jsonMap, _ := utils.JSONToMap(jsonBytes)
//	intent, err := domain.JSONToTradeIntent(jsonMap)
func JSONToTradeIntent(m map[string]interface{}) (*pb.TradeIntent, error) {
	return JSONToTradeIntentWithWhitelist(m, []string{"XAUUSD"})
}

// JSONToTradeIntentWithWhitelist convierte un map JSON a TradeIntent proto utilizando una whitelist dinámica.
//
// Si la whitelist está vacía se usa el fallback histórico `[]string{"XAUUSD"}` para mantener compatibilidad.
func JSONToTradeIntentWithWhitelist(m map[string]interface{}, whitelist []string) (*pb.TradeIntent, error) {
	intent, err := buildTradeIntentFromJSON(m)
 	if err != nil {
 		return nil, err
 	}

 	effective := whitelist
 	if len(effective) == 0 {
 		effective = []string{"XAUUSD"}
 	}

 	if err := ValidateTradeIntent(intent, effective); err != nil {
 		return nil, fmt.Errorf("validation failed: %w", err)
 	}

 	return intent, nil
}

func buildTradeIntentFromJSON(m map[string]interface{}) (*pb.TradeIntent, error) {
	// Extraer payload
	payload, ok := m["payload"].(map[string]interface{})
	if !ok {
		return nil, NewError(ErrMissingRequiredField, "payload not found or invalid")
	}

	// Construir proto
	intent := &pb.TradeIntent{
		TradeId:     utils.ExtractString(payload, "trade_id"),
		TimestampMs: utils.ExtractInt64(m, "timestamp_ms"),
		ClientId:    utils.ExtractString(payload, "client_id"),
		Symbol:      utils.ExtractString(payload, "symbol"),
		LotSize:     utils.ExtractFloat64(payload, "lot_size"),
		Price:       utils.ExtractFloat64(payload, "price"),
		MagicNumber: utils.ExtractInt64(payload, "magic_number"),
		Ticket:      int32(utils.ExtractInt64(payload, "ticket")),
	}

	// OrderSide: "BUY" o "SELL"
	orderSideStr := utils.ExtractString(payload, "order_side")
	switch orderSideStr {
	case "BUY":
		intent.Side = pb.OrderSide_ORDER_SIDE_BUY
	case "SELL":
		intent.Side = pb.OrderSide_ORDER_SIDE_SELL
	default:
		return nil, NewError(ErrInvalidPrice, fmt.Sprintf("invalid order_side: %s", orderSideStr))
	}

	// SL/TP opcionales
	if sl := utils.ExtractFloat64(payload, "stop_loss"); sl != 0 {
		intent.StopLoss = &sl
	}

	if tp := utils.ExtractFloat64(payload, "take_profit"); tp != 0 {
		intent.TakeProfit = &tp
	}

	// Comment opcional
	if comment := utils.ExtractString(payload, "comment"); comment != "" {
		intent.Comment = &comment
	}

	// Attempt opcional
	if attempt := int32(utils.ExtractInt64(payload, "attempt")); attempt != 0 {
		intent.Attempt = &attempt
	}

	if strategyID := utils.ExtractString(payload, "strategy_id"); strategyID != "" {
		intent.StrategyId = strategyID
	}

	// Timestamps (Issue #C1)
	intent.Timestamps = parseTimestamps(payload)

	return intent, nil
}

// TradeIntentToJSON convierte un TradeIntent proto a JSON map.
//
// Formato de salida compatible con Agent → EAs.
func TradeIntentToJSON(intent *pb.TradeIntent) (map[string]interface{}, error) {
	if intent == nil {
		return nil, NewError(ErrMissingRequiredField, "TradeIntent is nil")
	}

	orderSideStr := "BUY"
	if intent.Side == pb.OrderSide_ORDER_SIDE_SELL {
		orderSideStr = "SELL"
	}

	payload := map[string]interface{}{
		"trade_id":     intent.TradeId,
		"client_id":    intent.ClientId,
		"symbol":       intent.Symbol,
		"order_side":   orderSideStr,
		"lot_size":     intent.LotSize,
		"price":        intent.Price,
		"magic_number": intent.MagicNumber,
		"ticket":       intent.Ticket,
	}

	if intent.StrategyId != "" {
		payload["strategy_id"] = intent.StrategyId
	}

	// Opcionales
	if intent.StopLoss != nil {
		payload["stop_loss"] = *intent.StopLoss
	}
	if intent.TakeProfit != nil {
		payload["take_profit"] = *intent.TakeProfit
	}
	if intent.Comment != nil {
		payload["comment"] = *intent.Comment
	}
	if intent.Attempt != nil {
		payload["attempt"] = *intent.Attempt
	}

	// Timestamps (Issue #C1)
	if tsMap := timestampsToMap(intent.Timestamps); tsMap != nil {
		payload["timestamps"] = tsMap
	}

	result := map[string]interface{}{
		"type":         "trade_intent",
		"timestamp_ms": intent.TimestampMs,
		"payload":      payload,
	}

	return result, nil
}

// JSONToExecuteOrder convierte un map JSON a ExecuteOrder proto.
func JSONToExecuteOrder(m map[string]interface{}) (*pb.ExecuteOrder, error) {
	payload, ok := m["payload"].(map[string]interface{})
	if !ok {
		return nil, NewError(ErrMissingRequiredField, "payload not found")
	}

	order := &pb.ExecuteOrder{
		CommandId:       utils.ExtractString(payload, "command_id"),
		TradeId:         utils.ExtractString(payload, "trade_id"),
		TimestampMs:     utils.ExtractInt64(m, "timestamp_ms"),
		TargetClientId:  utils.ExtractString(payload, "target_client_id"),  // Issue #C5
		TargetAccountId: utils.ExtractString(payload, "target_account_id"), // Issue #C5
		Symbol:          utils.ExtractString(payload, "symbol"),
		LotSize:         utils.ExtractFloat64(payload, "lot_size"),
		MagicNumber:     utils.ExtractInt64(payload, "magic_number"),
	}

	// OrderSide
	orderSideStr := utils.ExtractString(payload, "order_side")
	switch orderSideStr {
	case "BUY":
		order.Side = pb.OrderSide_ORDER_SIDE_BUY
	case "SELL":
		order.Side = pb.OrderSide_ORDER_SIDE_SELL
	default:
		return nil, NewError(ErrInvalidPrice, fmt.Sprintf("invalid order_side: %s", orderSideStr))
	}

	// Opcionales
	if sl := utils.ExtractFloat64(payload, "stop_loss"); sl != 0 {
		order.StopLoss = &sl
	}
	if tp := utils.ExtractFloat64(payload, "take_profit"); tp != 0 {
		order.TakeProfit = &tp
	}
	if comment := utils.ExtractString(payload, "comment"); comment != "" {
		order.Comment = &comment
	}

	// Timestamps (Issue #C1)
	order.Timestamps = parseTimestamps(payload)

	if rawStops, ok := payload["adjustable_stops"]; ok {
		if stops := adjustableStopsFromJSON(rawStops); stops != nil {
			order.AdjustableStops = stops
		}
	}

	// Validar antes de retornar (Issue #A1)
	if err := ValidateExecuteOrder(order); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return order, nil
}

// ExecuteOrderToJSON convierte un ExecuteOrder proto a JSON map.
//
// Formato de salida para Agent → Slave EA.
func ExecuteOrderToJSON(order *pb.ExecuteOrder) (map[string]interface{}, error) {
	if order == nil {
		return nil, NewError(ErrMissingRequiredField, "ExecuteOrder is nil")
	}

	orderSideStr := "BUY"
	if order.Side == pb.OrderSide_ORDER_SIDE_SELL {
		orderSideStr = "SELL"
	}

	payload := map[string]interface{}{
		"command_id":        order.CommandId,
		"trade_id":          order.TradeId,
		"target_client_id":  order.TargetClientId,  // Issue #C5
		"target_account_id": order.TargetAccountId, // Issue #C5
		"symbol":            order.Symbol,
		"order_side":        orderSideStr,
		"lot_size":          order.LotSize,
		"magic_number":      order.MagicNumber,
	}

	// Opcionales
	if order.StopLoss != nil {
		payload["stop_loss"] = *order.StopLoss
	}
	if order.TakeProfit != nil {
		payload["take_profit"] = *order.TakeProfit
	}
	if order.Comment != nil {
		payload["comment"] = *order.Comment
	}

	// Timestamps (Issue #C1)
	if tsMap := timestampsToMap(order.Timestamps); tsMap != nil {
		payload["timestamps"] = tsMap
	}

	if order.AdjustableStops != nil {
		payload["adjustable_stops"] = adjustableStopsToJSON(order.AdjustableStops)
	}

	result := map[string]interface{}{
		"type":         "execute_order",
		"timestamp_ms": order.TimestampMs,
		"payload":      payload,
	}

	return result, nil
}

// JSONToExecutionResult convierte un map JSON a ExecutionResult proto.
func JSONToExecutionResult(m map[string]interface{}) (*pb.ExecutionResult, error) {
	payload, ok := m["payload"].(map[string]interface{})
	if !ok {
		return nil, NewError(ErrMissingRequiredField, "payload not found")
	}

	result := &pb.ExecutionResult{
		CommandId: utils.ExtractString(payload, "command_id"),
		TradeId:   utils.ExtractString(payload, "trade_id"), // Issue #C2
		Success:   utils.ExtractBool(payload, "success"),
		Ticket:    int32(utils.ExtractInt64(payload, "ticket")),
	}

	// ErrorCode: string → proto enum
	errorCodeStr := utils.ExtractString(payload, "error_code")
	if errorCodeStr != "" {
		result.ErrorCode = stringToProtoErrorCode(errorCodeStr)
	}

	// ErrorMessage opcional
	if errorMsg := utils.ExtractString(payload, "error_message"); errorMsg != "" {
		result.ErrorMessage = &errorMsg
	}

	// ExecutedPrice opcional
	if execPrice := utils.ExtractFloat64(payload, "executed_price"); execPrice != 0 {
		result.ExecutedPrice = &execPrice
	}

	// ExecutionTime opcional
	if execTime := utils.ExtractInt64(payload, "execution_time_ms"); execTime != 0 {
		result.ExecutionTimeMs = &execTime
	}

	// Timestamps (Issue #C1)
	result.Timestamps = parseTimestamps(payload)

	// Validar antes de retornar (Issue #A1)
	if err := ValidateExecutionResult(result); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return result, nil
}

// JSONToCloseResult convierte un map JSON (close_result) a ExecutionResult proto.
//
// Mapeo de campos:
//   - command_id → ExecutionResult.command_id
//   - trade_id (si viene) → ExecutionResult.trade_id
//   - success → ExecutionResult.success
//   - ticket → ExecutionResult.ticket
//   - error_code (string) → ExecutionResult.error_code (enum)
//   - error_message → ExecutionResult.error_message
//   - close_price → ExecutionResult.executed_price
//   - timestamp_ms (nivel raíz) → ExecutionResult.execution_time_ms
func JSONToCloseResult(m map[string]interface{}) (*pb.ExecutionResult, error) {
	payload, ok := m["payload"].(map[string]interface{})
	if !ok {
		return nil, NewError(ErrMissingRequiredField, "payload not found")
	}

	result := &pb.ExecutionResult{
		CommandId: utils.ExtractString(payload, "command_id"),
		TradeId:   utils.ExtractString(payload, "trade_id"),
		Success:   utils.ExtractBool(payload, "success"),
		Ticket:    int32(utils.ExtractInt64(payload, "ticket")),
	}

	// ErrorCode: string → proto enum
	errorCodeStr := utils.ExtractString(payload, "error_code")
	if errorCodeStr != "" {
		result.ErrorCode = stringToProtoErrorCode(errorCodeStr)
	}

	// ErrorMessage opcional
	if errorMsg := utils.ExtractString(payload, "error_message"); errorMsg != "" {
		result.ErrorMessage = &errorMsg
	}

	// ClosePrice → ExecutedPrice
	if closePrice := utils.ExtractFloat64(payload, "close_price"); closePrice != 0 {
		result.ExecutedPrice = &closePrice
	}

	// timestamp_ms del mensaje como execution_time_ms
	if ts := utils.ExtractInt64(m, "timestamp_ms"); ts != 0 {
		result.ExecutionTimeMs = &ts
	}

	// Timestamps (si vinieran)
	result.Timestamps = parseTimestamps(payload)

	// Validar antes de retornar
	if err := ValidateExecutionResult(result); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return result, nil
}

// ExecutionResultToJSON convierte un ExecutionResult proto a JSON map.
func ExecutionResultToJSON(result *pb.ExecutionResult) (map[string]interface{}, error) {
	if result == nil {
		return nil, NewError(ErrMissingRequiredField, "ExecutionResult is nil")
	}

	payload := map[string]interface{}{
		"command_id": result.CommandId,
		"trade_id":   result.TradeId, // Issue #C2
		"success":    result.Success,
		"ticket":     result.Ticket,
		"error_code": protoErrorCodeToString(result.ErrorCode),
	}

	// Opcionales
	if result.ErrorMessage != nil {
		payload["error_message"] = *result.ErrorMessage
	}
	if result.ExecutedPrice != nil {
		payload["executed_price"] = *result.ExecutedPrice
	}
	if result.ExecutionTimeMs != nil {
		payload["execution_time_ms"] = *result.ExecutionTimeMs
	}

	// Timestamps (Issue #C1)
	if tsMap := timestampsToMap(result.Timestamps); tsMap != nil {
		payload["timestamps"] = tsMap
	}

	return map[string]interface{}{
		"type":    "execution_result",
		"payload": payload,
	}, nil
}

// JSONToTradeClose convierte un map JSON a TradeClose proto.
func JSONToTradeClose(m map[string]interface{}) (*pb.TradeClose, error) {
	payload, ok := m["payload"].(map[string]interface{})
	if !ok {
		return nil, NewError(ErrMissingRequiredField, "payload not found")
	}

	close := &pb.TradeClose{
		TradeId:     utils.ExtractString(payload, "trade_id"),
		TimestampMs: utils.ExtractInt64(m, "timestamp_ms"),
		ClientId:    utils.ExtractString(payload, "client_id"),  // Issue #M4
		AccountId:   utils.ExtractString(payload, "account_id"), // Issue #M4
		Ticket:      int32(utils.ExtractInt64(payload, "ticket")),
		Symbol:      utils.ExtractString(payload, "symbol"),      // Issue #M4
		MagicNumber: utils.ExtractInt64(payload, "magic_number"), // Issue #M4
		ClosePrice:  utils.ExtractFloat64(payload, "close_price"),
	}

	// Opcionales
	if profit := utils.ExtractFloat64(payload, "profit"); profit != 0 {
		close.Profit = &profit
	}
	if reason := utils.ExtractString(payload, "reason"); reason != "" {
		close.Reason = &reason
	}

	// Validar antes de retornar (Issue #A1)
	if err := ValidateTradeClose(close); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return close, nil
}

// TradeCloseToJSON convierte un TradeClose proto a JSON map.
func TradeCloseToJSON(close *pb.TradeClose) (map[string]interface{}, error) {
	if close == nil {
		return nil, NewError(ErrMissingRequiredField, "TradeClose is nil")
	}

	payload := map[string]interface{}{
		"trade_id":     close.TradeId,
		"client_id":    close.ClientId,  // Issue #M4
		"account_id":   close.AccountId, // Issue #M4
		"ticket":       close.Ticket,
		"symbol":       close.Symbol,      // Issue #M4
		"magic_number": close.MagicNumber, // Issue #M4
		"close_price":  close.ClosePrice,
	}

	// Opcionales
	if close.Profit != nil {
		payload["profit"] = *close.Profit
	}
	if close.Reason != nil {
		payload["reason"] = *close.Reason
	}

	return map[string]interface{}{
		"type":         "trade_close",
		"timestamp_ms": close.TimestampMs,
		"payload":      payload,
	}, nil
}

// CloseOrderToJSON convierte un CloseOrder proto a JSON map.
//
// Formato de salida para Agent → Slave EA.
// Issue #C3: Implementación completa para flujo de cierre.
func CloseOrderToJSON(order *pb.CloseOrder) (map[string]interface{}, error) {
	if order == nil {
		return nil, NewError(ErrMissingRequiredField, "CloseOrder is nil")
	}

	payload := map[string]interface{}{
		"command_id":        order.CommandId,
		"trade_id":          order.TradeId,
		"target_client_id":  order.TargetClientId,  // Issue #C3
		"target_account_id": order.TargetAccountId, // Issue #C3
		"ticket":            order.Ticket,
		"symbol":            order.Symbol,      // Issue #C3
		"magic_number":      order.MagicNumber, // Issue #C3
	}

	// LotSize opcional
	if order.LotSize != nil {
		payload["lot_size"] = *order.LotSize
	}

	// Timestamps
	if tsMap := timestampsToMap(order.Timestamps); tsMap != nil {
		payload["timestamps"] = tsMap
	}

	return map[string]interface{}{
		"type":         "close_order",
		"timestamp_ms": order.TimestampMs,
		"payload":      payload,
	}, nil
}

// TradeIntentToExecuteOrder transforma un TradeIntent en ExecuteOrder.
//
// Aplica opciones de transformación (sizing, offsets, etc.).
//
// Example (Core):
//
//	intent := // ... TradeIntent recibido
//	opts := &domain.TransformOptions{
//	    LotSize:   0.10,  // Calculado por MM
//	    CommandID: utils.GenerateUUIDv7(),
//	    ClientID:  "slave_67890",
//	}
//	order := domain.TradeIntentToExecuteOrder(intent, opts)
func TradeIntentToExecuteOrder(intent *pb.TradeIntent, opts *TransformOptions) *pb.ExecuteOrder {
	if opts == nil {
		opts = NewTransformOptions()
	}

	order := &pb.ExecuteOrder{
		CommandId:       opts.CommandID,
		TradeId:         intent.TradeId,
		TimestampMs:     utils.NowUnixMilli(),
		TargetClientId:  opts.ClientID,  // Issue #C5
		TargetAccountId: opts.AccountID, // Issue #C5
		Symbol:          intent.Symbol,
		Side:            intent.Side,
		LotSize:         opts.LotSize,
		MagicNumber:     intent.MagicNumber,
		// Clonar timestamps para evitar estado compartido entre órdenes
		Timestamps: cloneTimestamps(intent.Timestamps),
	}

	// SL/TP con offsets
	if intent.StopLoss != nil && *intent.StopLoss != 0 {
		sl := *intent.StopLoss
		if opts.ApplyOffsets && opts.SLOffset != 0 {
			// Ajustar SL según side
			if intent.Side == pb.OrderSide_ORDER_SIDE_BUY {
				sl -= opts.SLOffset // BUY: SL más bajo
			} else {
				sl += opts.SLOffset // SELL: SL más alto
			}
		}
		order.StopLoss = &sl
	} else if opts.CatastrophicSL != nil {
		// Aplicar SL catastrófico si no hay SL del master
		order.StopLoss = opts.CatastrophicSL
	}

	if intent.TakeProfit != nil && *intent.TakeProfit != 0 {
		tp := *intent.TakeProfit
		if opts.ApplyOffsets && opts.TPOffset != 0 {
			// Ajustar TP según side
			if intent.Side == pb.OrderSide_ORDER_SIDE_BUY {
				tp += opts.TPOffset // BUY: TP más alto
			} else {
				tp -= opts.TPOffset // SELL: TP más bajo
			}
		}
		order.TakeProfit = &tp
	}

	// Comment opcional
	if intent.Comment != nil {
		order.Comment = intent.Comment
	}

	if opts.AdjustableStops != nil {
		order.AdjustableStops = opts.AdjustableStops.ToProto()
	}

	return order
}

// stringToProtoErrorCode convierte un string de error code a pb.ErrorCode.
func stringToProtoErrorCode(s string) pb.ErrorCode {
	switch s {
	case "NO_ERROR", "ERR_NO_ERROR":
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	case "INVALID_PRICE", "ERR_INVALID_PRICE":
		return pb.ErrorCode_ERROR_CODE_INVALID_PRICE
	case "INVALID_STOPS", "ERR_INVALID_STOPS":
		return pb.ErrorCode_ERROR_CODE_INVALID_STOPS
	case "INVALID_VOLUME", "ERR_INVALID_VOLUME":
		return pb.ErrorCode_ERROR_CODE_INVALID_VOLUME
	case "MARKET_CLOSED", "ERR_MARKET_CLOSED":
		return pb.ErrorCode_ERROR_CODE_MARKET_CLOSED
	case "NO_MONEY", "ERR_NO_MONEY", "ERR_NOT_ENOUGH_MONEY":
		return pb.ErrorCode_ERROR_CODE_NO_MONEY
	case "PRICE_CHANGED", "ERR_PRICE_CHANGED":
		return pb.ErrorCode_ERROR_CODE_PRICE_CHANGED
	case "OFF_QUOTES", "ERR_OFF_QUOTES":
		return pb.ErrorCode_ERROR_CODE_OFF_QUOTES
	case "BROKER_BUSY", "ERR_BROKER_BUSY":
		return pb.ErrorCode_ERROR_CODE_BROKER_BUSY
	case "REQUOTE", "ERR_REQUOTE":
		return pb.ErrorCode_ERROR_CODE_REQUOTE
	case "TOO_MANY_REQUESTS", "ERR_TOO_MANY_REQUESTS":
		return pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS
	case "TIMEOUT", "ERR_TIMEOUT":
		return pb.ErrorCode_ERROR_CODE_TIMEOUT
	case "TRADE_DISABLED", "ERR_TRADE_DISABLED":
		return pb.ErrorCode_ERROR_CODE_TRADE_DISABLED
	case "LONG_ONLY", "ERR_LONG_ONLY":
		return pb.ErrorCode_ERROR_CODE_LONG_ONLY
	case "SHORT_ONLY", "ERR_SHORT_ONLY":
		return pb.ErrorCode_ERROR_CODE_SHORT_ONLY
	case "SPEC_MISSING", "ERR_SPEC_MISSING":
		return pb.ErrorCode_ERROR_CODE_SPEC_MISSING
	case "RISK_POLICY_MISSING", "ERR_RISK_POLICY_MISSING":
		return pb.ErrorCode_ERROR_CODE_RISK_POLICY_MISSING
	default:
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
}

// protoErrorCodeToString convierte un pb.ErrorCode a string.
func protoErrorCodeToString(code pb.ErrorCode) string {
	switch code {
	case pb.ErrorCode_ERROR_CODE_UNSPECIFIED:
		return "NO_ERROR"
	case pb.ErrorCode_ERROR_CODE_INVALID_PRICE:
		return "ERR_INVALID_PRICE"
	case pb.ErrorCode_ERROR_CODE_INVALID_STOPS:
		return "ERR_INVALID_STOPS"
	case pb.ErrorCode_ERROR_CODE_INVALID_VOLUME:
		return "ERR_INVALID_VOLUME"
	case pb.ErrorCode_ERROR_CODE_MARKET_CLOSED:
		return "ERR_MARKET_CLOSED"
	case pb.ErrorCode_ERROR_CODE_NO_MONEY:
		return "ERR_NO_MONEY"
	case pb.ErrorCode_ERROR_CODE_PRICE_CHANGED:
		return "ERR_PRICE_CHANGED"
	case pb.ErrorCode_ERROR_CODE_OFF_QUOTES:
		return "ERR_OFF_QUOTES"
	case pb.ErrorCode_ERROR_CODE_BROKER_BUSY:
		return "ERR_BROKER_BUSY"
	case pb.ErrorCode_ERROR_CODE_REQUOTE:
		return "ERR_REQUOTE"
	case pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS:
		return "ERR_TOO_MANY_REQUESTS"
	case pb.ErrorCode_ERROR_CODE_TIMEOUT:
		return "ERR_TIMEOUT"
	case pb.ErrorCode_ERROR_CODE_TRADE_DISABLED:
		return "ERR_TRADE_DISABLED"
	case pb.ErrorCode_ERROR_CODE_LONG_ONLY:
		return "ERR_LONG_ONLY"
	case pb.ErrorCode_ERROR_CODE_SHORT_ONLY:
		return "ERR_SHORT_ONLY"
	case pb.ErrorCode_ERROR_CODE_SPEC_MISSING:
		return "ERR_SPEC_MISSING"
	case pb.ErrorCode_ERROR_CODE_RISK_POLICY_MISSING:
		return "ERR_RISK_POLICY_MISSING"
	default:
		return "ERR_UNKNOWN"
	}
}

// JSONToStateSnapshot convierte un map JSON a StateSnapshot proto.
func JSONToStateSnapshot(m map[string]interface{}) (*pb.StateSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("state snapshot payload is nil")
	}

	snapshot := &pb.StateSnapshot{}

	if ts := utils.ExtractInt64(m, "timestamp_ms"); ts > 0 {
		snapshot.TimestampMs = ts
	}

	accountsRaw, _ := m["accounts"].([]interface{})
	for _, rawAccount := range accountsRaw {
		accountMap, ok := rawAccount.(map[string]interface{})
		if !ok {
			continue
		}
		accountID := strings.TrimSpace(utils.ExtractString(accountMap, "account_id"))
		if accountID == "" {
			continue
		}

		info := &pb.AccountInfo{
			AccountId:   accountID,
			Balance:     utils.ExtractFloat64(accountMap, "balance"),
			Equity:      utils.ExtractFloat64(accountMap, "equity"),
			Margin:      utils.ExtractFloat64(accountMap, "margin"),
			MarginFree:  utils.ExtractFloat64(accountMap, "margin_free"),
			MarginLevel: utils.ExtractFloat64(accountMap, "margin_level"),
		}

		if ts := utils.ExtractInt64(accountMap, "timestamp_ms"); ts > 0 {
			info.TimestampMs = ts
		} else if snapshot.TimestampMs > 0 {
			info.TimestampMs = snapshot.TimestampMs
		}

		currency := strings.ToUpper(strings.TrimSpace(utils.ExtractString(accountMap, "currency")))
		if currency != "" {
			info.Currency = currency
		}

		snapshot.Accounts = append(snapshot.Accounts, info)
	}

	positionsRaw, _ := m["positions"].([]interface{})
	for _, rawPosition := range positionsRaw {
		posMap, ok := rawPosition.(map[string]interface{})
		if !ok {
			continue
		}

		ticket := int32(utils.ExtractInt64(posMap, "ticket"))
		if ticket == 0 {
			continue
		}

		position := &pb.PositionInfo{
			Ticket:      ticket,
			Symbol:      strings.TrimSpace(utils.ExtractString(posMap, "symbol")),
			Volume:      utils.ExtractFloat64(posMap, "volume"),
			OpenPrice:   utils.ExtractFloat64(posMap, "open_price"),
			Profit:      utils.ExtractFloat64(posMap, "profit"),
			MagicNumber: utils.ExtractInt64(posMap, "magic_number"),
		}

		switch strings.ToUpper(strings.TrimSpace(utils.ExtractString(posMap, "side"))) {
		case "BUY":
			position.Side = pb.OrderSide_ORDER_SIDE_BUY
		case "SELL":
			position.Side = pb.OrderSide_ORDER_SIDE_SELL
		default:
			position.Side = pb.OrderSide_ORDER_SIDE_UNSPECIFIED
		}

		if sl, ok := extractOptionalFloat(posMap, "stop_loss"); ok {
			position.StopLoss = &sl
		}
		if tp, ok := extractOptionalFloat(posMap, "take_profit"); ok {
			position.TakeProfit = &tp
		}

		if comment := strings.TrimSpace(utils.ExtractString(posMap, "comment")); comment != "" {
			position.Comment = &comment
		}

		if ts := utils.ExtractInt64(posMap, "open_time_ms"); ts > 0 {
			position.OpenTimeMs = ts
		}

		snapshot.Positions = append(snapshot.Positions, position)
	}

	return snapshot, nil
}

func extractOptionalFloat(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	value, exists := m[key]
	if !exists {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
