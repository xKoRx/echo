package domain

import (
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// TradeIntent representa un intent de trade enriquecido con validaciones.
//
// Embeds el mensaje proto y agrega funcionalidad de dominio.
type TradeIntent struct {
	*pb.TradeIntent
	validationErrors []error
}

// NewTradeIntent crea un nuevo TradeIntent desde un mensaje proto.
func NewTradeIntent(proto *pb.TradeIntent) *TradeIntent {
	return &TradeIntent{
		TradeIntent:      proto,
		validationErrors: make([]error, 0),
	}
}

// Validate ejecuta validaciones completas del TradeIntent.
//
// Retorna el primer error encontrado o nil si todo es válido.
func (t *TradeIntent) Validate(symbolWhitelist []string) error {
	return ValidateTradeIntent(t.TradeIntent, symbolWhitelist)
}

// AddValidationError agrega un error de validación al intent.
func (t *TradeIntent) AddValidationError(err error) {
	t.validationErrors = append(t.validationErrors, err)
}

// ValidationErrors retorna todos los errores de validación acumulados.
func (t *TradeIntent) ValidationErrors() []error {
	return t.validationErrors
}

// HasValidationErrors indica si hay errores de validación.
func (t *TradeIntent) HasValidationErrors() bool {
	return len(t.validationErrors) > 0
}

// IsValid indica si el intent es válido (sin errores).
func (t *TradeIntent) IsValid() bool {
	return !t.HasValidationErrors()
}

// ExecuteOrder representa un comando de ejecución enriquecido.
type ExecuteOrder struct {
	*pb.ExecuteOrder
	validationErrors []error
}

// NewExecuteOrder crea un nuevo ExecuteOrder desde un mensaje proto.
func NewExecuteOrder(proto *pb.ExecuteOrder) *ExecuteOrder {
	return &ExecuteOrder{
		ExecuteOrder:     proto,
		validationErrors: make([]error, 0),
	}
}

// Validate ejecuta validaciones completas del ExecuteOrder.
func (e *ExecuteOrder) Validate() error {
	return ValidateExecuteOrder(e.ExecuteOrder)
}

// AddValidationError agrega un error de validación.
func (e *ExecuteOrder) AddValidationError(err error) {
	e.validationErrors = append(e.validationErrors, err)
}

// ValidationErrors retorna todos los errores de validación.
func (e *ExecuteOrder) ValidationErrors() []error {
	return e.validationErrors
}

// HasValidationErrors indica si hay errores de validación.
func (e *ExecuteOrder) HasValidationErrors() bool {
	return len(e.validationErrors) > 0
}

// IsValid indica si el order es válido.
func (e *ExecuteOrder) IsValid() bool {
	return !e.HasValidationErrors()
}

// ExecutionResult representa un resultado de ejecución enriquecido.
type ExecutionResult struct {
	*pb.ExecutionResult
	validationErrors []error
}

// NewExecutionResult crea un nuevo ExecutionResult desde un mensaje proto.
func NewExecutionResult(proto *pb.ExecutionResult) *ExecutionResult {
	return &ExecutionResult{
		ExecutionResult:  proto,
		validationErrors: make([]error, 0),
	}
}

// Validate ejecuta validaciones del ExecutionResult.
func (e *ExecutionResult) Validate() error {
	return ValidateExecutionResult(e.ExecutionResult)
}

// AddValidationError agrega un error de validación.
func (e *ExecutionResult) AddValidationError(err error) {
	e.validationErrors = append(e.validationErrors, err)
}

// ValidationErrors retorna todos los errores de validación.
func (e *ExecutionResult) ValidationErrors() []error {
	return e.validationErrors
}

// HasValidationErrors indica si hay errores de validación.
func (e *ExecutionResult) HasValidationErrors() bool {
	return len(e.validationErrors) > 0
}

// IsValid indica si el result es válido.
func (e *ExecutionResult) IsValid() bool {
	return !e.HasValidationErrors()
}

// IsSuccess indica si la ejecución fue exitosa.
func (e *ExecutionResult) IsSuccess() bool {
	return e.ExecutionResult.Success
}

// TradeClose representa un evento de cierre de trade.
type TradeClose struct {
	*pb.TradeClose
}

// NewTradeClose crea un nuevo TradeClose desde un mensaje proto.
func NewTradeClose(proto *pb.TradeClose) *TradeClose {
	return &TradeClose{
		TradeClose: proto,
	}
}

// CloseOrder representa un comando de cierre de orden.
type CloseOrder struct {
	*pb.CloseOrder
}

// NewCloseOrder crea un nuevo CloseOrder desde un mensaje proto.
func NewCloseOrder(proto *pb.CloseOrder) *CloseOrder {
	return &CloseOrder{
		CloseOrder: proto,
	}
}

// ModifyOrder representa un comando de modificación de orden.
type ModifyOrder struct {
	*pb.ModifyOrder
}

// NewModifyOrder crea un nuevo ModifyOrder desde un mensaje proto.
func NewModifyOrder(proto *pb.ModifyOrder) *ModifyOrder {
	return &ModifyOrder{
		ModifyOrder: proto,
	}
}

// AccountInfo representa información de cuenta.
type AccountInfo struct {
	*pb.AccountInfo
}

// NewAccountInfo crea un nuevo AccountInfo desde un mensaje proto.
func NewAccountInfo(proto *pb.AccountInfo) *AccountInfo {
	return &AccountInfo{
		AccountInfo: proto,
	}
}

// Validate ejecuta validaciones del AccountInfo.
func (a *AccountInfo) Validate() error {
	return ValidateAccountInfo(a.AccountInfo)
}

// PositionInfo representa información de posición.
type PositionInfo struct {
	*pb.PositionInfo
}

// NewPositionInfo crea un nuevo PositionInfo desde un mensaje proto.
func NewPositionInfo(proto *pb.PositionInfo) *PositionInfo {
	return &PositionInfo{
		PositionInfo: proto,
	}
}

// SymbolInfo representa especificaciones de un símbolo.
type SymbolInfo struct {
	*pb.SymbolInfo
}

// NewSymbolInfo crea un nuevo SymbolInfo desde un mensaje proto.
func NewSymbolInfo(proto *pb.SymbolInfo) *SymbolInfo {
	return &SymbolInfo{
		SymbolInfo: proto,
	}
}

// Validate ejecuta validaciones del SymbolInfo.
func (s *SymbolInfo) Validate() error {
	return ValidateSymbolInfo(s.SymbolInfo)
}

// CalculateLotSize calcula el lot size basado en especificaciones.
//
// Ajusta al lot_step y clamps entre min_lot y max_lot.
func (s *SymbolInfo) CalculateLotSize(desiredLots float64) float64 {
	lots := desiredLots

	// Ajustar a lot_step
	if s.LotStep > 0 {
		lots = float64(int64(lots/s.LotStep)) * s.LotStep
	}

	// Clamp a rangos
	if lots < s.MinLot {
		lots = s.MinLot
	}
	if lots > s.MaxLot {
		lots = s.MaxLot
	}

	return lots
}

// TransformOptions opciones para transformaciones TradeIntent → ExecuteOrder.
//
// Usado por el Core para calcular sizing y aplicar políticas.
type TransformOptions struct {
	// LotSize calculado para el slave (después de MM)
	LotSize float64

	// CommandID único para este ExecuteOrder
	CommandID string

	// ClientID del slave destino
	ClientID string

	// AccountID del slave destino
	AccountID string

	// Offset para SL/TP (puntos)
	SLOffset float64
	TPOffset float64

	// Aplicar offsets a SL/TP
	ApplyOffsets bool

	// SL catastrófico (si está configurado)
	CatastrophicSL *float64

	// AdjustableStops anexados al comando
	AdjustableStops *AdjustableStops
}

// NewTransformOptions crea opciones con valores por defecto.
func NewTransformOptions() *TransformOptions {
	return &TransformOptions{
		ApplyOffsets: false,
	}
}

