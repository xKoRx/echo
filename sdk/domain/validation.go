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
// En i1 se espera formato UUIDv7 (RFC 9562).
// En i0 se aceptaba UUIDv4 por compatibilidad con MT4.
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

// ValidateUUIDv7 valida que un UUID sea formato v7 específicamente.
//
// UUIDv7 debe tener:
// - Formato UUID estándar (8-4-4-4-12)
// - Versión 7 en byte 6 (nibble alto)
// - Variant RFC 4122 en byte 8 (2 bits altos = 10)
//
// Example:
//
//	err := ValidateUUIDv7("01HKQ000-0000-7000-8000-000000000001")
//	// => nil (válido)
//
//	err := ValidateUUIDv7("550e8400-e29b-41d4-a716-446655440000")  // v4
//	// => error (no es v7)
func ValidateUUIDv7(uuid string) error {
	// Primero validar formato UUID básico
	if err := ValidateTradeID(uuid); err != nil {
		return err
	}

	// Extraer version nibble (posición 14, después del tercer guion)
	// Formato: xxxxxxxx-xxxx-Vxxx-xxxx-xxxxxxxxxxxx
	//                        ^
	//                     posición 14
	if len(uuid) < 15 {
		return NewValidationError("uuid", uuid, "UUID too short")
	}

	versionChar := uuid[14]
	if versionChar != '7' {
		return NewValidationError("uuid", uuid, fmt.Sprintf("not UUIDv7 (version nibble is '%c', expected '7')", versionChar))
	}

	// Extraer variant bits (posición 19, después del cuarto guion)
	// Formato: xxxxxxxx-xxxx-xxxx-Yxxx-xxxxxxxxxxxx
	//                             ^
	//                          posición 19
	// Variant RFC 4122 = 10xx en binario = 8, 9, A, B en hex
	if len(uuid) < 20 {
		return NewValidationError("uuid", uuid, "UUID too short for variant check")
	}

	variantChar := strings.ToUpper(string(uuid[19]))
	validVariants := []string{"8", "9", "A", "B"}
	isValidVariant := false
	for _, v := range validVariants {
		if variantChar == v {
			isValidVariant = true
			break
		}
	}

	if !isValidVariant {
		return NewValidationError("uuid", uuid, fmt.Sprintf("invalid UUID variant (nibble is '%s', expected 8/9/A/B)", variantChar))
	}

	return nil
}

// ValidateTradeIDv7 valida que un trade_id sea UUIDv7 específicamente (i1+).
//
// Wrapper sobre ValidateUUIDv7 para claridad semántica.
func ValidateTradeIDv7(tradeID string) error {
	if err := ValidateUUIDv7(tradeID); err != nil {
		return WrapError(ErrInvalidTradeID, "trade_id must be UUIDv7", err)
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

// NormalizeCanonical normaliza un símbolo a su forma canónica (i3).
//
// Reglas de normalización:
//  1. Uppercase y trim
//  2. Remover sufijos de broker conocidos: .m, .i, .raw, .ecn, .pro, .c (case-insensitive)
//  3. Permitir solo A-Z, 0-9, '/', '-', '_'
//  4. Longitud post-normalización: 3-20 caracteres
//
// Ejemplos:
//
//	"xauusd.m" → "XAUUSD"
//	"EUR/USD" → "EUR/USD"
//	"btc-usd" → "BTC-USD"
//	" Gold.ECN " → "GOLD"
//	"SP500.c" → "SP500"
func NormalizeCanonical(symbol string) (string, error) {
	if symbol == "" {
		return "", NewValidationError("symbol", symbol, "symbol cannot be empty")
	}

	// 1. Uppercase y trim
	normalized := strings.ToUpper(strings.TrimSpace(symbol))

	// 2. Remover sufijos de broker conocidos (case-insensitive)
	suffixes := []string{".m", ".i", ".raw", ".ecn", ".pro", ".c", ".M", ".I", ".RAW", ".ECN", ".PRO", ".C"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(normalized, suffix) {
			normalized = normalized[:len(normalized)-len(suffix)]
		}
	}

	// 3. Filtrar caracteres permitidos: A-Z, 0-9, '/', '-', '_'
	var builder strings.Builder
	for _, r := range normalized {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '/' || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	normalized = builder.String()

	// 4. Validar longitud post-normalización: 3-20
	if len(normalized) < 3 {
		return "", NewValidationError("symbol", symbol, fmt.Sprintf("normalized symbol too short (after normalization: %s, min length: 3)", normalized))
	}
	if len(normalized) > 20 {
		return "", NewValidationError("symbol", symbol, fmt.Sprintf("normalized symbol too long (after normalization: %s, max length: 20)", normalized))
	}

	if normalized == "" {
		return "", NewValidationError("symbol", symbol, "symbol normalized to empty string")
	}

	return normalized, nil
}

// ValidateCanonicalSymbol valida que un símbolo canónico esté en la whitelist (i3).
//
// Primero normaliza el símbolo usando NormalizeCanonical, luego verifica que esté en la lista.
func ValidateCanonicalSymbol(symbol string, whitelist []string) error {
	if symbol == "" {
		return NewValidationError("symbol", symbol, "symbol cannot be empty")
	}

	// Normalizar primero
	normalized, err := NormalizeCanonical(symbol)
	if err != nil {
		return err
	}

	// Verificar whitelist
	for _, allowed := range whitelist {
		allowedNormalized, err := NormalizeCanonical(allowed)
		if err != nil {
			continue // Skip invalid entries in whitelist
		}
		if allowedNormalized == normalized {
			return nil
		}
	}

	return NewValidationError("symbol", symbol, fmt.Sprintf("canonical symbol not in whitelist (normalized: %s)", normalized))
}

// ValidateSymbolMapping valida un SymbolMapping completo (i3).
//
// Reglas:
//   - Requeridos: broker_symbol y canonical_symbol
//   - Canonical en whitelist tras normalización
//   - Volúmenes: min_lot>0, max_lot>0, min_lot<=max_lot, lot_step>0
//   - Precio: digits>=0, point>0, tick_size>0
//   - Stop level: >=0 (0 = sin restricción)
//   - contract_size (opcional): si presente, >0
func ValidateSymbolMapping(mapping *pb.SymbolMapping, allowedCanonicals []string) error {
	if mapping == nil {
		return NewError(ErrMissingRequiredField, "SymbolMapping is nil")
	}

	// Validar campos requeridos
	if mapping.BrokerSymbol == "" {
		return NewValidationError("broker_symbol", "", "broker_symbol is required")
	}

	if mapping.CanonicalSymbol == "" {
		return NewValidationError("canonical_symbol", "", "canonical_symbol is required")
	}

	// Validar canonical en whitelist
	if err := ValidateCanonicalSymbol(mapping.CanonicalSymbol, allowedCanonicals); err != nil {
		normalized, _ := NormalizeCanonical(mapping.CanonicalSymbol)
		return fmt.Errorf("canonical_symbol %s not in whitelist (normalized: %s): %w", mapping.CanonicalSymbol, normalized, err)
	}

	// Validar volúmenes
	if mapping.MinLot <= 0 {
		return NewValidationError("min_lot", mapping.MinLot, "min_lot must be > 0")
	}

	if mapping.MaxLot <= 0 {
		return NewValidationError("max_lot", mapping.MaxLot, "max_lot must be > 0")
	}

	if mapping.MinLot > mapping.MaxLot {
		return NewValidationError("min_lot", mapping.MinLot, fmt.Sprintf("min_lot (%.2f) > max_lot (%.2f)", mapping.MinLot, mapping.MaxLot))
	}

	if mapping.LotStep <= 0 {
		return NewValidationError("lot_step", mapping.LotStep, "lot_step must be > 0")
	}

	// Validar precio
	if mapping.Digits < 0 {
		return NewValidationError("digits", mapping.Digits, "digits must be >= 0")
	}

	if mapping.Point <= 0 {
		return NewValidationError("point", mapping.Point, "point must be > 0")
	}

	if mapping.TickSize <= 0 {
		return NewValidationError("tick_size", mapping.TickSize, "tick_size must be > 0")
	}

	// Validar stop level
	if mapping.StopLevel < 0 {
		return NewValidationError("stop_level", mapping.StopLevel, "stop_level must be >= 0")
	}

	// Validar contract_size (opcional)
	if mapping.ContractSize != nil && *mapping.ContractSize <= 0 {
		return NewValidationError("contract_size", *mapping.ContractSize, "contract_size must be > 0")
	}

	return nil
}

// ValidateAccountSymbolsReport valida un AccountSymbolsReport completo (i3).
func ValidateAccountSymbolsReport(report *pb.AccountSymbolsReport, allowedCanonicals []string) error {
	if report == nil {
		return NewError(ErrMissingRequiredField, "AccountSymbolsReport is nil")
	}

	if err := ValidateAccountID(report.AccountId); err != nil {
		return fmt.Errorf("invalid account_id: %w", err)
	}

	if report.ReportedAtMs <= 0 {
		return NewValidationError("reported_at_ms", report.ReportedAtMs, "reported_at_ms must be positive")
	}

	// Validar cada mapping
	for i, mapping := range report.Symbols {
		if err := ValidateSymbolMapping(mapping, allowedCanonicals); err != nil {
			return fmt.Errorf("invalid symbol mapping at index %d: %w", i, err)
		}
	}

	return nil
}

// SymbolSpecValidationOptions define banderas opcionales para validar especificaciones de símbolo.
type SymbolSpecValidationOptions struct {
	RequireTickValue bool
}

// ValidateSymbolSpecReport valida un SymbolSpecReport completo.
func ValidateSymbolSpecReport(report *pb.SymbolSpecReport, allowedCanonicals []string) error {
	return ValidateSymbolSpecReportWithOptions(report, allowedCanonicals, SymbolSpecValidationOptions{})
}

// ValidateSymbolSpecReportWithOptions permite validar incluyendo opciones adicionales (i6).
func ValidateSymbolSpecReportWithOptions(report *pb.SymbolSpecReport, allowedCanonicals []string, opts SymbolSpecValidationOptions) error {
	if report == nil {
		return NewError(ErrMissingRequiredField, "SymbolSpecReport is nil")
	}

	if err := ValidateAccountID(report.AccountId); err != nil {
		return fmt.Errorf("invalid account_id: %w", err)
	}

	if report.ReportedAtMs <= 0 {
		return NewValidationError("reported_at_ms", report.ReportedAtMs, "reported_at_ms must be positive")
	}

	if len(report.Symbols) == 0 {
		return NewValidationError("symbols", 0, "symbols list cannot be empty")
	}

	for i, spec := range report.Symbols {
		if err := validateSymbolSpecification(spec, allowedCanonicals, opts); err != nil {
			return fmt.Errorf("invalid symbol specification at index %d: %w", i, err)
		}
	}

	return nil
}

// ValidateSymbolSpecification valida las especificaciones completas de un símbolo.
func ValidateSymbolSpecification(spec *pb.SymbolSpecification, allowedCanonicals []string) error {
	return validateSymbolSpecification(spec, allowedCanonicals, SymbolSpecValidationOptions{})
}

// ValidateSymbolSpecificationWithOptions permite validar especificaciones con opciones avanzadas.
func ValidateSymbolSpecificationWithOptions(spec *pb.SymbolSpecification, allowedCanonicals []string, opts SymbolSpecValidationOptions) error {
	return validateSymbolSpecification(spec, allowedCanonicals, opts)
}

func validateSymbolSpecification(spec *pb.SymbolSpecification, allowedCanonicals []string, opts SymbolSpecValidationOptions) error {
	if spec == nil {
		return NewError(ErrMissingRequiredField, "SymbolSpecification is nil")
	}

	if spec.BrokerSymbol == "" {
		return NewValidationError("broker_symbol", "", "broker_symbol is required")
	}

	if spec.CanonicalSymbol == "" {
		return NewValidationError("canonical_symbol", "", "canonical_symbol is required")
	}

	if err := ValidateCanonicalSymbol(spec.CanonicalSymbol, allowedCanonicals); err != nil {
		return fmt.Errorf("canonical_symbol validation failed: %w", err)
	}

	if spec.General == nil {
		return NewValidationError("general", nil, "general specification is required")
	}

	if err := validateSymbolGeneral(spec.General, opts.RequireTickValue); err != nil {
		return err
	}

	if spec.Volume == nil {
		return NewValidationError("volume", nil, "volume specification is required")
	}

	if err := validateVolumeSpec(spec.Volume); err != nil {
		return err
	}

	if spec.Swap == nil {
		return NewValidationError("swap", nil, "swap specification is required")
	}

	if err := validateSwapSpec(spec.Swap); err != nil {
		return err
	}

	for i, session := range spec.Sessions {
		if err := validateSessionWindow(session); err != nil {
			return fmt.Errorf("invalid session window at index %d: %w", i, err)
		}
	}

	return nil
}

func validateSymbolGeneral(general *pb.SymbolGeneral, requireTickValue bool) error {
	if general == nil {
		return NewValidationError("general", nil, "general specification is required")
	}

	if general.SpreadType == pb.SpreadType_SPREAD_TYPE_UNSPECIFIED {
		return NewValidationError("spread_type", general.SpreadType, "spread_type cannot be unspecified")
	}

	if general.SpreadType == pb.SpreadType_SPREAD_TYPE_FIXED && general.FixedSpreadPoints <= 0 {
		return NewValidationError("fixed_spread_points", general.FixedSpreadPoints, "fixed_spread_points must be > 0 for fixed spread")
	}

	if general.Digits < 0 {
		return NewValidationError("digits", general.Digits, "digits must be >= 0")
	}

	if general.StopsLevel < 0 {
		return NewValidationError("stops_level", general.StopsLevel, "stops_level must be >= 0")
	}

	if general.ContractSize <= 0 {
		return NewValidationError("contract_size", general.ContractSize, "contract_size must be > 0")
	}

	if general.MarginCurrency == "" {
		return NewValidationError("margin_currency", "", "margin_currency is required")
	}

	if general.ProfitCalculationMode == pb.ProfitCalculationMode_PROFIT_CALCULATION_MODE_UNSPECIFIED {
		return NewValidationError("profit_calculation_mode", general.ProfitCalculationMode, "profit_calculation_mode cannot be unspecified")
	}

	if general.MarginCalculationMode == pb.MarginCalculationMode_MARGIN_CALCULATION_MODE_UNSPECIFIED {
		return NewValidationError("margin_calculation_mode", general.MarginCalculationMode, "margin_calculation_mode cannot be unspecified")
	}

	if general.MarginHedge < 0 {
		return NewValidationError("margin_hedge", general.MarginHedge, "margin_hedge must be >= 0")
	}

	if general.MarginPercentage <= 0 {
		return NewValidationError("margin_percentage", general.MarginPercentage, "margin_percentage must be > 0")
	}

	if general.TradePermission == pb.TradePermission_TRADE_PERMISSION_UNSPECIFIED {
		return NewValidationError("trade_permission", general.TradePermission, "trade_permission cannot be unspecified")
	}

	if general.ExecutionMode == pb.ExecutionMode_EXECUTION_MODE_UNSPECIFIED {
		return NewValidationError("execution_mode", general.ExecutionMode, "execution_mode cannot be unspecified")
	}

	if general.GtcMode == pb.GTCMode_GTC_MODE_UNSPECIFIED {
		return NewValidationError("gtc_mode", general.GtcMode, "gtc_mode cannot be unspecified")
	}

	if general.TickValue < 0 {
		return NewValidationError("tick_value", general.TickValue, "tick_value cannot be negative")
	}

	if requireTickValue && general.TickValue <= 0 {
		return NewValidationError("tick_value", general.TickValue, "tick_value must be > 0 when spec_report/tickvalue capability is present")
	}

	return nil
}

func validateVolumeSpec(volume *pb.VolumeSpec) error {
	if volume == nil {
		return NewValidationError("volume", nil, "volume specification is required")
	}

	if volume.MinVolume <= 0 {
		return NewValidationError("min_volume", volume.MinVolume, "min_volume must be > 0")
	}

	if volume.MaxVolume <= 0 {
		return NewValidationError("max_volume", volume.MaxVolume, "max_volume must be > 0")
	}

	if volume.MinVolume > volume.MaxVolume {
		return NewValidationError("min_volume", volume.MinVolume, fmt.Sprintf("min_volume (%.4f) > max_volume (%.4f)", volume.MinVolume, volume.MaxVolume))
	}

	if volume.VolumeStep <= 0 {
		return NewValidationError("volume_step", volume.VolumeStep, "volume_step must be > 0")
	}

	return nil
}

func validateSwapSpec(swap *pb.SwapSpec) error {
	if swap == nil {
		return NewValidationError("swap", nil, "swap specification is required")
	}

	if swap.SwapType == pb.SwapType_SWAP_TYPE_UNSPECIFIED {
		return NewValidationError("swap_type", swap.SwapType, "swap_type cannot be unspecified")
	}

	if swap.TripleSwapDay != pb.Weekday_WEEKDAY_UNSPECIFIED && (swap.TripleSwapDay < pb.Weekday_WEEKDAY_SUNDAY || swap.TripleSwapDay > pb.Weekday_WEEKDAY_SATURDAY) {
		return NewValidationError("triple_swap_day", swap.TripleSwapDay, "triple_swap_day must be a valid weekday")
	}

	return nil
}

func validateSessionWindow(window *pb.SessionWindow) error {
	if window == nil {
		return NewValidationError("sessions", nil, "session window is required")
	}

	if window.Day == pb.Weekday_WEEKDAY_UNSPECIFIED {
		return NewValidationError("day", window.Day, "session day cannot be unspecified")
	}

	for i, rng := range window.QuoteSessions {
		if err := validateSessionRange("quote_sessions", i, rng); err != nil {
			return err
		}
	}

	for i, rng := range window.TradeSessions {
		if err := validateSessionRange("trade_sessions", i, rng); err != nil {
			return err
		}
	}

	return nil
}

func validateSessionRange(field string, index int, rng *pb.SessionRange) error {
	if rng == nil {
		return NewValidationError(field, nil, fmt.Sprintf("session range at index %d is nil", index))
	}

	if rng.StartMinute >= 1440 || rng.EndMinute > 1440 {
		return NewValidationError(field, fmt.Sprintf("%d-%d", rng.StartMinute, rng.EndMinute), fmt.Sprintf("session range at index %d exceeds 24h", index))
	}

	if rng.StartMinute >= rng.EndMinute {
		return NewValidationError(field, fmt.Sprintf("%d-%d", rng.StartMinute, rng.EndMinute), fmt.Sprintf("session range at index %d must have start < end", index))
	}

	return nil
}

// ValidateSymbolQuoteSnapshot valida un SymbolQuoteSnapshot.
func ValidateSymbolQuoteSnapshot(snapshot *pb.SymbolQuoteSnapshot, allowedCanonicals []string) error {
	if snapshot == nil {
		return NewError(ErrMissingRequiredField, "SymbolQuoteSnapshot is nil")
	}

	if err := ValidateAccountID(snapshot.AccountId); err != nil {
		return fmt.Errorf("invalid account_id: %w", err)
	}

	if snapshot.CanonicalSymbol == "" {
		return NewValidationError("canonical_symbol", "", "canonical_symbol is required")
	}

	if err := ValidateCanonicalSymbol(snapshot.CanonicalSymbol, allowedCanonicals); err != nil {
		return fmt.Errorf("canonical_symbol validation failed: %w", err)
	}

	if snapshot.BrokerSymbol == "" {
		return NewValidationError("broker_symbol", "", "broker_symbol is required")
	}

	if snapshot.Bid <= 0 {
		return NewValidationError("bid", snapshot.Bid, "bid must be > 0")
	}

	if snapshot.Ask <= 0 {
		return NewValidationError("ask", snapshot.Ask, "ask must be > 0")
	}

	if snapshot.Ask < snapshot.Bid {
		return NewValidationError("ask", snapshot.Ask, "ask must be >= bid")
	}

	if snapshot.SpreadPoints < 0 {
		return NewValidationError("spread_points", snapshot.SpreadPoints, "spread_points must be >= 0")
	}

	if snapshot.TimestampMs <= 0 {
		return NewValidationError("timestamp_ms", snapshot.TimestampMs, "timestamp_ms must be positive")
	}

	return nil
}
