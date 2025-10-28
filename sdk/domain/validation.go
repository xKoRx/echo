package domain

import (
	"fmt"
	"regexp"
	"strings"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// ValidationError representa un error de validación.
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

// Error implementa la interfaz error.
func (v *ValidationError) Error() string {
	return fmt.Sprintf("validation error: field '%s' with value '%v': %s", v.Field, v.Value, v.Message)
}

// NewValidationError crea un nuevo ValidationError.
func NewValidationError(field string, value interface{}, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// Validaciones de símbolo

// ValidateSymbol valida que un símbolo esté en la whitelist.
//
// En i0 solo se permite XAUUSD.
//
// Example:
//
//	err := domain.ValidateSymbol("XAUUSD", []string{"XAUUSD"})
//	if err != nil {
//	    // Símbolo no permitido
//	}
func ValidateSymbol(symbol string, whitelist []string) error {
	if symbol == "" {
		return NewValidationError("symbol", symbol, "symbol cannot be empty")
	}

	// Normalizar a mayúsculas
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	// Verificar whitelist
	for _, allowed := range whitelist {
		if strings.ToUpper(allowed) == symbol {
			return nil
		}
	}

	return NewValidationError("symbol", symbol, fmt.Sprintf("symbol not in whitelist: %v", whitelist))
}

// ValidateSymbolFormat valida el formato básico de un símbolo.
//
// Formato esperado: 2-10 caracteres alfanuméricos en mayúsculas.
func ValidateSymbolFormat(symbol string) error {
	if symbol == "" {
		return NewValidationError("symbol", symbol, "symbol cannot be empty")
	}

	// Solo alfanuméricos, guiones y puntos
	matched, err := regexp.MatchString(`^[A-Z0-9._-]{2,15}$`, strings.ToUpper(symbol))
	if err != nil {
		return WrapError(ErrInvalidSymbol, "regex validation failed", err)
	}

	if !matched {
		return NewValidationError("symbol", symbol, "invalid symbol format (expected: 2-15 alphanumeric chars)")
	}

	return nil
}

// Validaciones de volumen/lot size

// ValidateLotSize valida que el lot size esté dentro de rangos permitidos.
//
// Example:
//
//	err := domain.ValidateLotSize(0.10, 0.01, 100.0, 0.01)
//	// => nil (válido)
func ValidateLotSize(lotSize, minLot, maxLot, lotStep float64) error {
	if lotSize <= 0 {
		return NewValidationError("lot_size", lotSize, "lot size must be positive")
	}

	if lotSize < minLot {
		return NewValidationError("lot_size", lotSize, fmt.Sprintf("lot size below minimum: %f", minLot))
	}

	if lotSize > maxLot {
		return NewValidationError("lot_size", lotSize, fmt.Sprintf("lot size exceeds maximum: %f", maxLot))
	}

	// Verificar que sea múltiplo de lotStep
	if lotStep > 0 {
		remainder := lotSize / lotStep
		if remainder != float64(int64(remainder)) {
			return NewValidationError("lot_size", lotSize, fmt.Sprintf("lot size must be multiple of %f", lotStep))
		}
	}

	return nil
}

// ValidateLotSizeBasic valida solo que el lot size sea positivo.
//
// Útil para i0 donde no se tienen specs completos.
func ValidateLotSizeBasic(lotSize float64) error {
	if lotSize <= 0 {
		return NewValidationError("lot_size", lotSize, "lot size must be positive")
	}
	return nil
}

// Validaciones de precio

// ValidatePrice valida que un precio sea positivo.
func ValidatePrice(price float64) error {
	if price <= 0 {
		return NewValidationError("price", price, "price must be positive")
	}
	return nil
}

// ValidatePriceRange valida que un precio esté dentro de un rango.
func ValidatePriceRange(price, min, max float64) error {
	if err := ValidatePrice(price); err != nil {
		return err
	}

	if price < min {
		return NewValidationError("price", price, fmt.Sprintf("price below minimum: %f", min))
	}

	if price > max {
		return NewValidationError("price", price, fmt.Sprintf("price exceeds maximum: %f", max))
	}

	return nil
}

// Validaciones de SL/TP

// ValidateStopLoss valida un stop loss respecto al precio de entrada.
//
// Para BUY: SL debe ser < precio
// Para SELL: SL debe ser > precio
func ValidateStopLoss(sl, entryPrice float64, side pb.OrderSide) error {
	if sl == 0 {
		// SL opcional en i0
		return nil
	}

	if err := ValidatePrice(sl); err != nil {
		return err
	}

	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		if sl >= entryPrice {
			return NewValidationError("stop_loss", sl, fmt.Sprintf("BUY stop loss must be below entry price: %f", entryPrice))
		}
	case pb.OrderSide_ORDER_SIDE_SELL:
		if sl <= entryPrice {
			return NewValidationError("stop_loss", sl, fmt.Sprintf("SELL stop loss must be above entry price: %f", entryPrice))
		}
	default:
		return NewValidationError("side", side, "invalid order side for SL validation")
	}

	return nil
}

// ValidateTakeProfit valida un take profit respecto al precio de entrada.
//
// Para BUY: TP debe ser > precio
// Para SELL: TP debe ser < precio
func ValidateTakeProfit(tp, entryPrice float64, side pb.OrderSide) error {
	if tp == 0 {
		// TP opcional en i0
		return nil
	}

	if err := ValidatePrice(tp); err != nil {
		return err
	}

	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		if tp <= entryPrice {
			return NewValidationError("take_profit", tp, fmt.Sprintf("BUY take profit must be above entry price: %f", entryPrice))
		}
	case pb.OrderSide_ORDER_SIDE_SELL:
		if tp >= entryPrice {
			return NewValidationError("take_profit", tp, fmt.Sprintf("SELL take profit must be below entry price: %f", entryPrice))
		}
	default:
		return NewValidationError("side", side, "invalid order side for TP validation")
	}

	return nil
}

// Validaciones de identificadores

// ValidateTradeID valida que un trade_id no esté vacío y tenga formato válido.
//
// En i0 se espera formato UUIDv7.
func ValidateTradeID(tradeID string) error {
	if tradeID == "" {
		return NewValidationError("trade_id", tradeID, "trade_id cannot be empty")
	}

	// Validar formato UUID básico (8-4-4-4-12 caracteres hex)
	matched, err := regexp.MatchString(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`, strings.ToUpper(tradeID))
	if err != nil {
		return WrapError(ErrInvalidTradeID, "regex validation failed", err)
	}

	if !matched {
		return NewValidationError("trade_id", tradeID, "invalid UUID format")
	}

	return nil
}

// ValidateCommandID valida que un command_id no esté vacío.
func ValidateCommandID(commandID string) error {
	if commandID == "" {
		return NewValidationError("command_id", commandID, "command_id cannot be empty")
	}

	// Mismo formato que trade_id
	return ValidateTradeID(commandID)
}

// ValidateMagicNumber valida que un magic number sea válido.
//
// MagicNumbers en MT4/MT5 son int64 no-negativos.
func ValidateMagicNumber(magicNumber int64) error {
	if magicNumber < 0 {
		return NewValidationError("magic_number", magicNumber, "magic_number cannot be negative")
	}

	// En MT4 el rango típico es 0 - 2^31-1 pero permitimos int64 completo
	return nil
}

// ValidateTicket valida que un ticket sea positivo.
func ValidateTicket(ticket int32) error {
	if ticket <= 0 {
		return NewValidationError("ticket", ticket, "ticket must be positive")
	}
	return nil
}

// Validaciones de OrderSide

// ValidateOrderSide valida que el lado de la orden sea válido.
func ValidateOrderSide(side pb.OrderSide) error {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY, pb.OrderSide_ORDER_SIDE_SELL:
		return nil
	case pb.OrderSide_ORDER_SIDE_UNSPECIFIED:
		return NewValidationError("side", side, "order side cannot be UNSPECIFIED")
	default:
		return NewValidationError("side", side, "invalid order side")
	}
}

// Validaciones compuestas (mensajes proto)

// ValidateTradeIntent valida un TradeIntent completo.
//
// Reglas i0:
// - trade_id, symbol, side, lot_size, price, magic_number obligatorios
// - symbol debe estar en whitelist (solo XAUUSD)
// - lot_size > 0
// - price > 0
func ValidateTradeIntent(intent *pb.TradeIntent, symbolWhitelist []string) error {
	if intent == nil {
		return NewError(ErrMissingRequiredField, "TradeIntent is nil")
	}

	// Validar campos obligatorios
	if err := ValidateTradeID(intent.TradeId); err != nil {
		return err
	}

	if err := ValidateSymbol(intent.Symbol, symbolWhitelist); err != nil {
		return err
	}

	if err := ValidateOrderSide(intent.Side); err != nil {
		return err
	}

	if err := ValidateLotSizeBasic(intent.LotSize); err != nil {
		return err
	}

	if err := ValidatePrice(intent.Price); err != nil {
		return err
	}

	if err := ValidateMagicNumber(intent.MagicNumber); err != nil {
		return err
	}

	if err := ValidateTicket(intent.Ticket); err != nil {
		return err
	}

	// Validar SL/TP opcionales
	if intent.StopLoss != nil && *intent.StopLoss != 0 {
		if err := ValidateStopLoss(*intent.StopLoss, intent.Price, intent.Side); err != nil {
			return err
		}
	}

	if intent.TakeProfit != nil && *intent.TakeProfit != 0 {
		if err := ValidateTakeProfit(*intent.TakeProfit, intent.Price, intent.Side); err != nil {
			return err
		}
	}

	return nil
}

// ValidateExecuteOrder valida un ExecuteOrder completo.
func ValidateExecuteOrder(order *pb.ExecuteOrder) error {
	if order == nil {
		return NewError(ErrMissingRequiredField, "ExecuteOrder is nil")
	}

	if err := ValidateCommandID(order.CommandId); err != nil {
		return err
	}

	if err := ValidateTradeID(order.TradeId); err != nil {
		return err
	}

	if err := ValidateSymbolFormat(order.Symbol); err != nil {
		return err
	}

	if err := ValidateOrderSide(order.Side); err != nil {
		return err
	}

	if err := ValidateLotSizeBasic(order.LotSize); err != nil {
		return err
	}

	if err := ValidateMagicNumber(order.MagicNumber); err != nil {
		return err
	}

	return nil
}

// ValidateExecutionResult valida un ExecutionResult.
func ValidateExecutionResult(result *pb.ExecutionResult) error {
	if result == nil {
		return NewError(ErrMissingRequiredField, "ExecutionResult is nil")
	}

	if err := ValidateCommandID(result.CommandId); err != nil {
		return err
	}

	// Si success=true, ticket debe ser positivo
	if result.Success && result.Ticket <= 0 {
		return NewValidationError("ticket", result.Ticket, "ticket must be positive when success=true")
	}

	return nil
}

// ValidateAccountInfo valida AccountInfo.
func ValidateAccountInfo(info *pb.AccountInfo) error {
	if info == nil {
		return NewError(ErrMissingRequiredField, "AccountInfo is nil")
	}

	if info.AccountId == "" {
		return NewValidationError("account_id", "", "account_id cannot be empty")
	}

	if info.Balance < 0 {
		return NewValidationError("balance", info.Balance, "balance cannot be negative")
	}

	if info.Equity < 0 {
		return NewValidationError("equity", info.Equity, "equity cannot be negative")
	}

	return nil
}

// ValidateSymbolInfo valida SymbolInfo.
func ValidateSymbolInfo(info *pb.SymbolInfo) error {
	if info == nil {
		return NewError(ErrMissingRequiredField, "SymbolInfo is nil")
	}

	if info.BrokerSymbol == "" {
		return NewValidationError("broker_symbol", "", "broker_symbol cannot be empty")
	}

	if info.CanonicalSymbol == "" {
		return NewValidationError("canonical_symbol", "", "canonical_symbol cannot be empty")
	}

	if info.Digits < 0 || info.Digits > 10 {
		return NewValidationError("digits", info.Digits, "digits must be between 0 and 10")
	}

	if info.Point <= 0 {
		return NewValidationError("point", info.Point, "point must be positive")
	}

	if info.MinLot <= 0 {
		return NewValidationError("min_lot", info.MinLot, "min_lot must be positive")
	}

	if info.MaxLot <= 0 {
		return NewValidationError("max_lot", info.MaxLot, "max_lot must be positive")
	}

	if info.MinLot > info.MaxLot {
		return NewValidationError("min_lot", info.MinLot, "min_lot cannot exceed max_lot")
	}

	if info.LotStep <= 0 {
		return NewValidationError("lot_step", info.LotStep, "lot_step must be positive")
	}

	return nil
}

// ValidateTradeClose valida un TradeClose completo.
//
// Reglas i0:
// - trade_id obligatorio
// - ticket debe ser positivo
// - close_price debe ser positivo
func ValidateTradeClose(close *pb.TradeClose) error {
	if close == nil {
		return NewError(ErrMissingRequiredField, "TradeClose is nil")
	}

	// Validar trade_id
	if err := ValidateTradeID(close.TradeId); err != nil {
		return err
	}

	// Validar ticket
	if err := ValidateTicket(close.Ticket); err != nil {
		return err
	}

	// Validar close_price
	if err := ValidatePrice(close.ClosePrice); err != nil {
		return err
	}

	return nil
}
