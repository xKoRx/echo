package semconv

import "go.opentelemetry.io/otel/attribute"

// Echo contiene atributos semánticos específicos de Echo Trade Copier.
//
// # Identificadores
//
//   - echo.trade_id: UUID del trade (UUIDv7)
//   - echo.command_id: UUID del comando ExecuteOrder
//   - echo.client_id: ID del cliente (master_XXX o slave_XXX)
//   - echo.account_id: ID de la cuenta MT4/MT5
//
// # Trading
//
//   - echo.symbol: Símbolo del instrumento (XAUUSD, etc.)
//   - echo.order_side: Lado de la orden (BUY/SELL)
//   - echo.lot_size: Tamaño en lotes
//   - echo.price: Precio de la orden
//   - echo.magic_number: MagicNumber MT4/MT5
//   - echo.ticket: Ticket MT4/MT5
//
// # Estado
//
//   - echo.status: Estado de ejecución (success/rejected/timeout)
//   - echo.error_code: Código de error si aplica
//   - echo.component: Componente (agent/core/master_ea/slave_ea)
//
// # Uso
//
//	import "github.com/xKoRx/echo/sdk/telemetry/semconv"
//
//	// Logs
//	client.Info(ctx, "Intent received",
//	    semconv.Echo.TradeID.String("01HKQV8Y..."),
//	    semconv.Echo.Symbol.String("XAUUSD"),
//	    semconv.Echo.OrderSide.String("BUY"),
//	)
//
//	// Métricas
//	metrics.RecordIntentReceived(ctx,
//	    semconv.Echo.TradeID.String("01HKQV8Y..."),
//	    semconv.Echo.ClientID.String("master_12345"),
//	)
var Echo = echoAttributes{
	// Identificadores
	TradeID:   attribute.Key("echo.trade_id"),
	CommandID: attribute.Key("echo.command_id"),
	ClientID:  attribute.Key("echo.client_id"),
	AccountID: attribute.Key("echo.account_id"),

	// Trading
	Symbol:      attribute.Key("echo.symbol"),
	OrderSide:   attribute.Key("echo.order_side"),
	LotSize:     attribute.Key("echo.lot_size"),
	Price:       attribute.Key("echo.price"),
	MagicNumber: attribute.Key("echo.magic_number"),
	Ticket:      attribute.Key("echo.ticket"),

	// Estado
	Status:    attribute.Key("echo.status"),
	ErrorCode: attribute.Key("echo.error_code"),
	Component: attribute.Key("echo.component"),

	// Adicionales
	Strategy:             attribute.Key("echo.strategy"),
	Timeframe:            attribute.Key("echo.timeframe"),
	ATR:                  attribute.Key("echo.atr"),
	SlipPage:             attribute.Key("echo.slippage"),
	Spread:               attribute.Key("echo.spread"),
	PipeCount:            attribute.Key("echo.pipe_count"),
	Attempt:              attribute.Key("echo.attempt"),
	Decision:             attribute.Key("echo.decision"),
	Reason:               attribute.Key("echo.reason"),
	PolicyType:           attribute.Key("echo.policy_type"),
	RiskAmount:           attribute.Key("echo.risk.amount"),
	RiskCurrency:         attribute.Key("echo.risk.currency"),
	RiskDecision:         attribute.Key("echo.risk.decision"),
	RiskRejectReason:     attribute.Key("echo.risk.reject_reason"),
	RiskCommissionPerLot: attribute.Key("echo.risk.commission_per_lot"),
	RiskCommissionTotal:  attribute.Key("echo.risk.commission_total"),
	RiskCommissionRate:   attribute.Key("echo.risk.commission_rate"),
}

type echoAttributes struct {
	// Identificadores
	TradeID   attribute.Key // UUID del trade (UUIDv7)
	CommandID attribute.Key // UUID del comando ExecuteOrder
	ClientID  attribute.Key // ID del cliente (master_XXX o slave_XXX)
	AccountID attribute.Key // ID de cuenta MT4/MT5

	// Trading
	Symbol      attribute.Key // Símbolo del instrumento
	OrderSide   attribute.Key // Lado de la orden (BUY/SELL)
	LotSize     attribute.Key // Tamaño en lotes
	Price       attribute.Key // Precio de la orden
	MagicNumber attribute.Key // MagicNumber MT4/MT5
	Ticket      attribute.Key // Ticket MT4/MT5

	// Estado
	Status    attribute.Key // Estado (success/rejected/timeout)
	ErrorCode attribute.Key // Código de error
	Component attribute.Key // Componente (agent/core/master_ea/slave_ea)

	// Adicionales
	Strategy             attribute.Key // ID de estrategia
	Timeframe            attribute.Key // Timeframe (M1, M5, H1, etc.)
	ATR                  attribute.Key // Average True Range
	SlipPage             attribute.Key // Slippage en points
	Spread               attribute.Key // Spread en points
	PipeCount            attribute.Key // Número de pipes activos
	Attempt              attribute.Key // Número de intento (reintentos)
	Decision             attribute.Key // Decisión (clamp/reject/pass_through)
	Reason               attribute.Key // Razón asociada a la decisión
	PolicyType           attribute.Key // Tipo de política de riesgo
	RiskAmount           attribute.Key // Monto de riesgo configurado
	RiskCurrency         attribute.Key // Divisa del riesgo
	RiskDecision         attribute.Key // Decisión del motor de riesgo (proceed/reject/fallback)
	RiskRejectReason     attribute.Key // Motivo del rechazo del riesgo
	RiskCommissionPerLot attribute.Key // Comisión por lote considerada en el cálculo de riesgo
	RiskCommissionTotal  attribute.Key // Comisión total estimada para el lote calculado
	RiskCommissionRate   attribute.Key // Comisión porcentual aplicada al valor de la orden
}

// Values pre-definidos para atributos comunes

// ComponentValues valores válidos para echo.component
var ComponentValues = struct {
	Agent    string
	Core     string
	MasterEA string
	SlaveEA  string
}{
	Agent:    "agent",
	Core:     "core",
	MasterEA: "master_ea",
	SlaveEA:  "slave_ea",
}

// OrderSideValues valores válidos para echo.order_side
var OrderSideValues = struct {
	Buy  string
	Sell string
}{
	Buy:  "BUY",
	Sell: "SELL",
}

// StatusValues valores válidos para echo.status
var StatusValues = struct {
	Success  string
	Rejected string
	Timeout  string
	Pending  string
}{
	Success:  "success",
	Rejected: "rejected",
	Timeout:  "timeout",
	Pending:  "pending",
}

// TimeframeValues valores válidos para echo.timeframe
var TimeframeValues = struct {
	M1  string
	M5  string
	M15 string
	M30 string
	H1  string
	H4  string
	D1  string
	W1  string
	MN1 string
}{
	M1:  "M1",
	M5:  "M5",
	M15: "M15",
	M30: "M30",
	H1:  "H1",
	H4:  "H4",
	D1:  "D1",
	W1:  "W1",
	MN1: "MN1",
}

// Helper functions para crear atributos comunes

// TradeAttributes crea un conjunto de atributos para un trade.
//
// Example:
//
//	attrs := semconv.TradeAttributes("01HKQV8Y...", "XAUUSD", "BUY")
//	client.Info(ctx, "Trade intent received", attrs...)
func TradeAttributes(tradeID, symbol, orderSide string) []attribute.KeyValue {
	return []attribute.KeyValue{
		Echo.TradeID.String(tradeID),
		Echo.Symbol.String(symbol),
		Echo.OrderSide.String(orderSide),
	}
}

// ExecutionAttributes crea atributos para una ejecución.
//
// Example:
//
//	attrs := semconv.ExecutionAttributes("cmd123", "01HKQV8Y...", "success")
//	client.Info(ctx, "Execution completed", attrs...)
func ExecutionAttributes(commandID, tradeID, status string) []attribute.KeyValue {
	return []attribute.KeyValue{
		Echo.CommandID.String(commandID),
		Echo.TradeID.String(tradeID),
		Echo.Status.String(status),
	}
}

// ClientAttributes crea atributos para un cliente.
//
// Example:
//
//	attrs := semconv.ClientAttributes("master_12345", "12345")
//	client.Info(ctx, "Client connected", attrs...)
func ClientAttributes(clientID, accountID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		Echo.ClientID.String(clientID),
		Echo.AccountID.String(accountID),
	}
}

// ErrorAttributes crea atributos para un error.
//
// Example:
//
//	attrs := semconv.ErrorAttributes("ERR_NO_MONEY", "rejected")
//	client.Error(ctx, "Execution failed", attrs...)
func ErrorAttributes(errorCode, status string) []attribute.KeyValue {
	return []attribute.KeyValue{
		Echo.ErrorCode.String(errorCode),
		Echo.Status.String(status),
	}
}
