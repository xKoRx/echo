// Package domain provee modelos de dominio y lógica de negocio para Echo.
package domain

import (
	"time"
)

// OrderStatus representa el estado de una orden en el sistema.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"   // Intención recibida, no enviada aún
	OrderStatusSent      OrderStatus = "SENT"      // Comando enviado al slave
	OrderStatusFilled    OrderStatus = "FILLED"    // Confirmación de fill recibida
	OrderStatusRejected  OrderStatus = "REJECTED"  // Rechazada por slave/broker
	OrderStatusCancelled OrderStatus = "CANCELLED" // Cancelada manualmente
)

// OrderSide representa la dirección de una orden.
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// Trade representa una intención de trade desde un Master.
// Corresponde a la tabla `echo.trades` en PostgreSQL.
type Trade struct {
	// Identidad
	TradeID         string      `json:"trade_id" db:"trade_id"`                   // UUIDv7 único
	SourceMasterID  string      `json:"source_master_id" db:"source_master_id"`   // ID del Master EA
	MasterAccountID string      `json:"master_account_id" db:"master_account_id"` // Account ID del master
	MasterTicket    int32       `json:"master_ticket" db:"master_ticket"`         // Ticket del master

	// Detalles del trade
	MagicNumber int64     `json:"magic_number" db:"magic_number"` // MagicNumber MT4/MT5
	Symbol      string    `json:"symbol" db:"symbol"`             // Símbolo (ej: XAUUSD)
	Side        OrderSide `json:"side" db:"side"`                 // BUY/SELL
	LotSize     float64   `json:"lot_size" db:"lot_size"`         // Tamaño en lotes
	Price       float64   `json:"price" db:"price"`               // Precio de apertura en master

	// SL/TP opcionales
	StopLoss   *float64 `json:"stop_loss,omitempty" db:"stop_loss"`     // Opcional
	TakeProfit *float64 `json:"take_profit,omitempty" db:"take_profit"` // Opcional
	Comment    *string  `json:"comment,omitempty" db:"comment"`         // Opcional

	// Estado
	Status  OrderStatus `json:"status" db:"status"`   // Estado actual
	Attempt int32       `json:"attempt" db:"attempt"` // Número de intento

	// Timestamps
	OpenedAtMs int64     `json:"opened_at_ms" db:"opened_at_ms"` // Timestamp de apertura en master
	CreatedAt  time.Time `json:"created_at" db:"created_at"`     // Timestamp de creación en BD
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`     // Timestamp de última actualización
}

// Execution representa una ejecución de orden en un slave.
// Corresponde a la tabla `echo.executions` en PostgreSQL.
type Execution struct {
	// Identidad
	ExecutionID string `json:"execution_id" db:"execution_id"` // UUID del execution (command_id)
	TradeID     string `json:"trade_id" db:"trade_id"`         // FK a trades

	// Slave info
	SlaveAccountID string `json:"slave_account_id" db:"slave_account_id"` // Account ID del slave
	AgentID        string `json:"agent_id" db:"agent_id"`                 // ID del agent que ejecutó

	// Resultado de ejecución
	SlaveTicket    int32    `json:"slave_ticket" db:"slave_ticket"`         // Ticket generado en slave (0 si fallo)
	ExecutedPrice  *float64 `json:"executed_price,omitempty" db:"executed_price"` // Precio ejecutado (NULL si fallo)
	Success        bool     `json:"success" db:"success"`                   // true = fill, false = reject
	ErrorCode      string   `json:"error_code" db:"error_code"`             // Código de error
	ErrorMessage   string   `json:"error_message" db:"error_message"`       // Mensaje de error

	// Latencia E2E (timestamps t0..t7)
	TimestampsMs map[string]int64 `json:"timestamps_ms" db:"timestamps_ms"` // JSONB con t0..t7

	// Timestamps
	CreatedAt time.Time `json:"created_at" db:"created_at"` // Timestamp de creación
}

// DedupeEntry representa una entrada de deduplicación.
// Corresponde a la tabla `echo.dedupe` en PostgreSQL.
type DedupeEntry struct {
	TradeID   string      `json:"trade_id" db:"trade_id"`   // UUIDv7 del trade
	Status    OrderStatus `json:"status" db:"status"`       // Estado actual
	CreatedAt time.Time   `json:"created_at" db:"created_at"` // Timestamp de creación
	UpdatedAt time.Time   `json:"updated_at" db:"updated_at"` // Timestamp de última actualización
}

// Close representa un cierre de posición en un slave.
// Corresponde a la tabla `echo.closes` en PostgreSQL.
type Close struct {
	// Identidad
	CloseID string `json:"close_id" db:"close_id"` // UUID del close command
	TradeID string `json:"trade_id" db:"trade_id"` // FK a trades

	// Slave info
	SlaveAccountID string `json:"slave_account_id" db:"slave_account_id"` // Account ID del slave
	SlaveTicket    int32  `json:"slave_ticket" db:"slave_ticket"`         // Ticket cerrado en slave

	// Resultado de cierre
	ClosePrice   *float64 `json:"close_price,omitempty" db:"close_price"` // Precio de cierre (NULL si fallo)
	Success      bool     `json:"success" db:"success"`                   // true = cerrado, false = error
	ErrorCode    string   `json:"error_code" db:"error_code"`             // Código de error
	ErrorMessage string   `json:"error_message" db:"error_message"`       // Mensaje de error

	// Timestamps
	ClosedAtMs int64     `json:"closed_at_ms" db:"closed_at_ms"` // Timestamp de cierre
	CreatedAt  time.Time `json:"created_at" db:"created_at"`     // Timestamp de creación
}

// LatencyMetrics representa métricas de latencia E2E calculadas desde timestamps.
type LatencyMetrics struct {
	// Latencias por hop (en milisegundos)
	MasterToAgentMs int64 `json:"master_to_agent_ms"` // t1 - t0
	AgentToCoreMs   int64 `json:"agent_to_core_ms"`   // t2 - t1
	CoreProcessMs   int64 `json:"core_process_ms"`    // t3 - t2
	CoreToAgentMs   int64 `json:"core_to_agent_ms"`   // t4 - t3
	AgentToSlaveMs  int64 `json:"agent_to_slave_ms"`  // t5 - t4
	SlaveProcessMs  int64 `json:"slave_process_ms"`   // t6 - t5
	OrderFillMs     int64 `json:"order_fill_ms"`      // t7 - t6

	// Latencia E2E
	E2EMs int64 `json:"e2e_ms"` // t7 - t0

	// Timestamps raw (para debugging)
	T0 int64 `json:"t0"` // Master EA genera intent
	T1 int64 `json:"t1"` // Agent recibe de pipe
	T2 int64 `json:"t2"` // Core recibe de stream
	T3 int64 `json:"t3"` // Core envía ExecuteOrder
	T4 int64 `json:"t4"` // Agent recibe ExecuteOrder
	T5 int64 `json:"t5"` // Slave EA recibe comando
	T6 int64 `json:"t6"` // Slave EA llama OrderSend
	T7 int64 `json:"t7"` // Slave EA recibe ticket/fill
}

// CalculateLatency calcula métricas de latencia desde un mapa de timestamps.
//
// Retorna nil si los timestamps están incompletos o inválidos.
func CalculateLatency(timestamps map[string]int64) *LatencyMetrics {
	// Validar que existan todos los timestamps
	required := []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6", "t7"}
	for _, key := range required {
		if _, ok := timestamps[key]; !ok {
			return nil
		}
	}

	t0 := timestamps["t0"]
	t1 := timestamps["t1"]
	t2 := timestamps["t2"]
	t3 := timestamps["t3"]
	t4 := timestamps["t4"]
	t5 := timestamps["t5"]
	t6 := timestamps["t6"]
	t7 := timestamps["t7"]

	// Validar que los timestamps sean crecientes (con tolerancia de clock skew)
	// En i1 permitimos valores negativos por problemas de GetTickCount en MT4
	// TODO i2: mejorar sincronización de relojes

	return &LatencyMetrics{
		MasterToAgentMs: t1 - t0,
		AgentToCoreMs:   t2 - t1,
		CoreProcessMs:   t3 - t2,
		CoreToAgentMs:   t4 - t3,
		AgentToSlaveMs:  t5 - t4,
		SlaveProcessMs:  t6 - t5,
		OrderFillMs:     t7 - t6,
		E2EMs:           t7 - t0,
		T0:              t0,
		T1:              t1,
		T2:              t2,
		T3:              t3,
		T4:              t4,
		T5:              t5,
		T6:              t6,
		T7:              t7,
	}
}

// IsTerminal indica si un OrderStatus es terminal (no cambiará más).
func (s OrderStatus) IsTerminal() bool {
	return s == OrderStatusFilled || s == OrderStatusRejected || s == OrderStatusCancelled
}

// String implementa fmt.Stringer para OrderStatus.
func (s OrderStatus) String() string {
	return string(s)
}

// String implementa fmt.Stringer para OrderSide.
func (s OrderSide) String() string {
	return string(s)
}

