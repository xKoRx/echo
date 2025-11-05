package domain

import (
	"fmt"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// ErrorCode representa un código de error del dominio de trading.
type ErrorCode string

// Códigos de error estándar
const (
	// ErrNoError indica éxito (sin error)
	ErrNoError ErrorCode = "NO_ERROR"

	// Errores de validación
	ErrInvalidPrice        ErrorCode = "INVALID_PRICE"
	ErrInvalidStops        ErrorCode = "INVALID_STOPS"
	ErrInvalidVolume       ErrorCode = "INVALID_VOLUME"
	ErrInvalidSymbol       ErrorCode = "INVALID_SYMBOL"
	ErrInvalidMagicNumber  ErrorCode = "INVALID_MAGIC_NUMBER"
	ErrInvalidTradeID      ErrorCode = "INVALID_TRADE_ID"
	ErrInvalidCommandID    ErrorCode = "INVALID_COMMAND_ID"
	ErrMissingRequiredField ErrorCode = "MISSING_REQUIRED_FIELD"

	// Errores de mercado/broker
	ErrMarketClosed    ErrorCode = "MARKET_CLOSED"
	ErrNoMoney         ErrorCode = "NO_MONEY"
	ErrPriceChanged    ErrorCode = "PRICE_CHANGED"
	ErrOffQuotes       ErrorCode = "OFF_QUOTES"
	ErrBrokerBusy      ErrorCode = "BROKER_BUSY"
	ErrRequote         ErrorCode = "REQUOTE"
	ErrTooManyRequests ErrorCode = "TOO_MANY_REQUESTS"
	ErrTimeout         ErrorCode = "TIMEOUT"
	ErrTradeDisabled   ErrorCode = "TRADE_DISABLED"
	ErrLongOnly        ErrorCode = "LONG_ONLY"
	ErrShortOnly       ErrorCode = "SHORT_ONLY"

	// Errores de sistema
	ErrUnknown           ErrorCode = "UNKNOWN"
	ErrConnectionLost    ErrorCode = "CONNECTION_LOST"
	ErrDuplicateTrade    ErrorCode = "DUPLICATE_TRADE"
	ErrNotFound          ErrorCode = "NOT_FOUND"
	ErrDedupeConflict    ErrorCode = "DEDUPE_CONFLICT"
	ErrPolicyViolation   ErrorCode = "POLICY_VIOLATION"
	ErrWindowBlocked     ErrorCode = "WINDOW_BLOCKED"
	ErrSpreadExceeded    ErrorCode = "SPREAD_EXCEEDED"
	ErrSlippageExceeded  ErrorCode = "SLIPPAGE_EXCEEDED"
	ErrDelayExceeded     ErrorCode = "DELAY_EXCEEDED"
	ErrSpecMissing       ErrorCode = "SPEC_MISSING"
	ErrRiskPolicyMissing ErrorCode = "RISK_POLICY_MISSING"
)

// TradingError representa un error del dominio de trading con contexto.
type TradingError struct {
	Code    ErrorCode
	Message string
	Details map[string]interface{}
	Wrapped error
}

// Error implementa la interfaz error.
func (e *TradingError) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Wrapped)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap implementa la interfaz errors.Unwrap.
func (e *TradingError) Unwrap() error {
	return e.Wrapped
}

// WithDetail agrega un detalle al error.
func (e *TradingError) WithDetail(key string, value interface{}) *TradingError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// NewError crea un nuevo TradingError.
//
// Example:
//
//	err := domain.NewError(domain.ErrInvalidSymbol, "Symbol XAUUSD not allowed")
func NewError(code ErrorCode, message string) *TradingError {
	return &TradingError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WrapError envuelve un error existente con contexto de trading.
//
// Example:
//
//	err := domain.WrapError(domain.ErrConnectionLost, "gRPC connection failed", originalErr)
func WrapError(code ErrorCode, message string, wrapped error) *TradingError {
	return &TradingError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
		Wrapped: wrapped,
	}
}

// ErrorCodeFromProto convierte un pb.ErrorCode a domain.ErrorCode.
func ErrorCodeFromProto(protoCode pb.ErrorCode) ErrorCode {
	switch protoCode {
	case pb.ErrorCode_ERROR_CODE_UNSPECIFIED:
		return ErrUnknown
	case pb.ErrorCode_ERROR_CODE_INVALID_PRICE:
		return ErrInvalidPrice
	case pb.ErrorCode_ERROR_CODE_INVALID_STOPS:
		return ErrInvalidStops
	case pb.ErrorCode_ERROR_CODE_INVALID_VOLUME:
		return ErrInvalidVolume
	case pb.ErrorCode_ERROR_CODE_MARKET_CLOSED:
		return ErrMarketClosed
	case pb.ErrorCode_ERROR_CODE_NO_MONEY:
		return ErrNoMoney
	case pb.ErrorCode_ERROR_CODE_PRICE_CHANGED:
		return ErrPriceChanged
	case pb.ErrorCode_ERROR_CODE_OFF_QUOTES:
		return ErrOffQuotes
	case pb.ErrorCode_ERROR_CODE_BROKER_BUSY:
		return ErrBrokerBusy
	case pb.ErrorCode_ERROR_CODE_REQUOTE:
		return ErrRequote
	case pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS:
		return ErrTooManyRequests
	case pb.ErrorCode_ERROR_CODE_TIMEOUT:
		return ErrTimeout
	case pb.ErrorCode_ERROR_CODE_TRADE_DISABLED:
		return ErrTradeDisabled
	case pb.ErrorCode_ERROR_CODE_LONG_ONLY:
		return ErrLongOnly
	case pb.ErrorCode_ERROR_CODE_SHORT_ONLY:
		return ErrShortOnly
	case pb.ErrorCode_ERROR_CODE_SPEC_MISSING:
		return ErrSpecMissing
	case pb.ErrorCode_ERROR_CODE_RISK_POLICY_MISSING:
		return ErrRiskPolicyMissing
	default:
		return ErrUnknown
	}
}

// ErrorCodeToProto convierte un domain.ErrorCode a pb.ErrorCode.
func ErrorCodeToProto(code ErrorCode) pb.ErrorCode {
	switch code {
	case ErrNoError:
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	case ErrInvalidPrice:
		return pb.ErrorCode_ERROR_CODE_INVALID_PRICE
	case ErrInvalidStops:
		return pb.ErrorCode_ERROR_CODE_INVALID_STOPS
	case ErrInvalidVolume:
		return pb.ErrorCode_ERROR_CODE_INVALID_VOLUME
	case ErrMarketClosed:
		return pb.ErrorCode_ERROR_CODE_MARKET_CLOSED
	case ErrNoMoney:
		return pb.ErrorCode_ERROR_CODE_NO_MONEY
	case ErrPriceChanged:
		return pb.ErrorCode_ERROR_CODE_PRICE_CHANGED
	case ErrOffQuotes:
		return pb.ErrorCode_ERROR_CODE_OFF_QUOTES
	case ErrBrokerBusy:
		return pb.ErrorCode_ERROR_CODE_BROKER_BUSY
	case ErrRequote:
		return pb.ErrorCode_ERROR_CODE_REQUOTE
	case ErrTooManyRequests:
		return pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS
	case ErrTimeout:
		return pb.ErrorCode_ERROR_CODE_TIMEOUT
	case ErrTradeDisabled:
		return pb.ErrorCode_ERROR_CODE_TRADE_DISABLED
	case ErrLongOnly:
		return pb.ErrorCode_ERROR_CODE_LONG_ONLY
	case ErrShortOnly:
		return pb.ErrorCode_ERROR_CODE_SHORT_ONLY
	case ErrSpecMissing:
		return pb.ErrorCode_ERROR_CODE_SPEC_MISSING
	case ErrRiskPolicyMissing:
		return pb.ErrorCode_ERROR_CODE_RISK_POLICY_MISSING
	default:
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
}

// ErrorCodeString retorna una representación string del error code.
func ErrorCodeString(code ErrorCode) string {
	return string(code)
}

// IsRetryable indica si un error es retriable (puede reintentarse).
func IsRetryable(code ErrorCode) bool {
	switch code {
	case ErrBrokerBusy, ErrRequote, ErrTimeout, ErrTooManyRequests, ErrOffQuotes:
		return true
	default:
		return false
	}
}

// IsFatal indica si un error es fatal (no se debe reintentar).
func IsFatal(code ErrorCode) bool {
	switch code {
	case ErrInvalidSymbol, ErrInvalidMagicNumber, ErrInvalidTradeID,
		ErrInvalidCommandID, ErrMissingRequiredField, ErrDuplicateTrade:
		return true
	default:
		return false
	}
}

// ErrorFromMT4Code convierte un código de error MT4 a ErrorCode.
//
// Códigos MT4 comunes:
// - 129: ERR_INVALID_PRICE
// - 130: ERR_INVALID_STOPS
// - 131: ERR_INVALID_TRADE_VOLUME
// - 132: ERR_MARKET_CLOSED
// - 133: ERR_TRADE_DISABLED
// - 134: ERR_NOT_ENOUGH_MONEY
// - 135: ERR_PRICE_CHANGED
// - 136: ERR_OFF_QUOTES
// - 137: ERR_BROKER_BUSY
// - 138: ERR_REQUOTE
// - 141: ERR_TOO_MANY_REQUESTS
func ErrorFromMT4Code(mt4Code int) ErrorCode {
	switch mt4Code {
	case 0:
		return ErrNoError
	case 129:
		return ErrInvalidPrice
	case 130:
		return ErrInvalidStops
	case 131:
		return ErrInvalidVolume
	case 132:
		return ErrMarketClosed
	case 133:
		return ErrTradeDisabled
	case 134:
		return ErrNoMoney
	case 135:
		return ErrPriceChanged
	case 136:
		return ErrOffQuotes
	case 137:
		return ErrBrokerBusy
	case 138:
		return ErrRequote
	case 141:
		return ErrTooManyRequests
	case 4108: // ERR_UNKNOWN_TICKET (no standard pero común)
		return ErrNotFound
	default:
		return ErrUnknown
	}
}

