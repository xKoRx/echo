// Package domain provee interfaces de repositorio para persistencia.
package domain

import (
	"context"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// TradeRepository define operaciones de persistencia para Trade.
//
// Implementaciones:
//   - PostgreSQL: en core/internal/repository/trade_postgres.go
//
// Uso:
//
//	repo := NewTradeRepository(db)
//	err := repo.Create(ctx, trade)
type TradeRepository interface {
	// Create inserta un nuevo trade.
	// Retorna error si el trade_id ya existe.
	Create(ctx context.Context, trade *Trade) error

	// GetByID obtiene un trade por su trade_id.
	// Retorna nil si no existe.
	GetByID(ctx context.Context, tradeID string) (*Trade, error)

	// GetByMasterTicket obtiene un trade por master_ticket.
	// Útil para correlación con eventos del Master EA.
	GetByMasterTicket(ctx context.Context, masterAccountID string, masterTicket int32) (*Trade, error)

	// UpdateStatus actualiza el estado de un trade.
	UpdateStatus(ctx context.Context, tradeID string, status OrderStatus) error

	// List obtiene trades con paginación.
	// Retorna slice de trades ordenados por created_at DESC.
	List(ctx context.Context, limit, offset int) ([]*Trade, error)

	// ListByStatus obtiene trades por estado.
	ListByStatus(ctx context.Context, status OrderStatus, limit, offset int) ([]*Trade, error)
}

// ExecutionRepository define operaciones de persistencia para Execution.
type ExecutionRepository interface {
	// Create inserta una nueva ejecución.
	Create(ctx context.Context, exec *Execution) error

	// GetByID obtiene una ejecución por su execution_id.
	GetByID(ctx context.Context, executionID string) (*Execution, error)

	// GetByTradeID obtiene todas las ejecuciones de un trade.
	// Retorna slice ordenado por created_at ASC.
	GetByTradeID(ctx context.Context, tradeID string) ([]*Execution, error)

	// GetByTradeAndSlave obtiene la ejecución de un trade en un slave específico.
	// Retorna nil si no existe.
	GetByTradeAndSlave(ctx context.Context, tradeID, slaveAccountID string) (*Execution, error)

	// GetTicketByTradeAndSlave obtiene el ticket de un trade en un slave.
	// Retorna 0 si no existe o si la ejecución falló.
	// Útil para resolver CloseOrder con ticket=0 (correlación).
	GetTicketByTradeAndSlave(ctx context.Context, tradeID, slaveAccountID string) (int32, error)

	// List obtiene ejecuciones con paginación.
	List(ctx context.Context, limit, offset int) ([]*Execution, error)

	// ListBySuccess obtiene ejecuciones exitosas o fallidas.
	ListBySuccess(ctx context.Context, success bool, limit, offset int) ([]*Execution, error)
}

// DedupeRepository define operaciones de persistencia para deduplicación.
type DedupeRepository interface {
	// Upsert inserta o actualiza una entrada de dedupe.
	// Si el trade_id ya existe, actualiza el status.
	Upsert(ctx context.Context, entry *DedupeEntry) error

	// Get obtiene una entrada de dedupe por trade_id.
	// Retorna nil si no existe.
	Get(ctx context.Context, tradeID string) (*DedupeEntry, error)

	// Exists verifica si un trade_id ya fue procesado.
	Exists(ctx context.Context, tradeID string) (bool, error)

	// UpdateStatus actualiza el status de una entrada de dedupe.
	UpdateStatus(ctx context.Context, tradeID string, status OrderStatus) error

	// CleanupTTL elimina entries con estados terminales más antiguos de TTL.
	// Retorna el número de entries eliminados.
	CleanupTTL(ctx context.Context) (int, error)
}

// CloseRepository define operaciones de persistencia para Close.
type CloseRepository interface {
	// Create inserta un nuevo cierre.
	Create(ctx context.Context, close *Close) error

	// GetByID obtiene un cierre por su close_id.
	GetByID(ctx context.Context, closeID string) (*Close, error)

	// GetByTradeID obtiene todos los cierres de un trade.
	// Retorna slice ordenado por created_at ASC.
	GetByTradeID(ctx context.Context, tradeID string) ([]*Close, error)

	// GetByTradeAndSlave obtiene el cierre de un trade en un slave específico.
	// Retorna nil si no existe.
	GetByTradeAndSlave(ctx context.Context, tradeID, slaveAccountID string) (*Close, error)

	// List obtiene cierres con paginación.
	List(ctx context.Context, limit, offset int) ([]*Close, error)
}

// CorrelationService define operaciones para correlación trade_id ↔ tickets.
//
// Este servicio encapsula la lógica de correlación determinística:
//   - Al abrir: registrar tickets por slave en executions
//   - Al cerrar: resolver ticket exacto por trade_id + slave_account_id
//
// Uso en Core:
//
//	tickets, err := correlationSvc.GetTicketsByTrade(ctx, tradeID)
//	ticket := tickets["67890"]  // ticket del slave 67890
type CorrelationService interface {
	// GetTicketsByTrade obtiene el mapa de slave_account_id → ticket para un trade.
	// Retorna solo ejecuciones exitosas (success=true, ticket!=0).
	GetTicketsByTrade(ctx context.Context, tradeID string) (map[string]int32, error)

	// GetTicketForSlave obtiene el ticket de un trade en un slave específico.
	// Retorna 0 si no existe o si falló la ejecución.
	GetTicketForSlave(ctx context.Context, tradeID, slaveAccountID string) (int32, error)

	// RecordExecution registra una ejecución (llamado tras recibir ExecutionResult).
	// Crea entrada en executions y actualiza dedupe status.
	RecordExecution(ctx context.Context, exec *Execution) error

	// RecordClose registra un cierre (llamado tras recibir CloseResult).
	RecordClose(ctx context.Context, close *Close) error
}

// SymbolRepository define operaciones de persistencia para mapeos de símbolos (i3).
type SymbolRepository interface {
	// UpsertAccountMapping inserta o actualiza mapeos de símbolos para una cuenta.
	// reportedAtMs se usa para idempotencia temporal (solo actualiza si es más reciente).
	UpsertAccountMapping(ctx context.Context, accountID string, mappings []*SymbolMapping, reportedAtMs int64) error

	// GetAccountMapping obtiene todos los mapeos de una cuenta.
	// Retorna un mapa canonical_symbol → AccountSymbolInfo.
	GetAccountMapping(ctx context.Context, accountID string) (map[string]*AccountSymbolInfo, error)

	// InvalidateAccount elimina todos los mapeos de una cuenta.
	// Se llama cuando una cuenta se desconecta.
	InvalidateAccount(ctx context.Context, accountID string) error
}

// SymbolSpecRepository define operaciones para persistir especificaciones de símbolos.
type SymbolSpecRepository interface {
	UpsertSpecifications(ctx context.Context, accountID string, specs []*pb.SymbolSpecification, reportedAtMs int64) error
	GetSpecifications(ctx context.Context, accountID string) (map[string]*pb.SymbolSpecification, error)
}

// SymbolQuoteRepository define operaciones para snapshots de precios.
type SymbolQuoteRepository interface {
	InsertSnapshot(ctx context.Context, snapshot *pb.SymbolQuoteSnapshot) error
	GetLatestSnapshot(ctx context.Context, accountID, canonicalSymbol string) (*pb.SymbolQuoteSnapshot, error)
}

// RiskPolicyRepository define operaciones para políticas de riesgo.
type RiskPolicyRepository interface {
	Get(ctx context.Context, accountID, strategyID string) (*RiskPolicy, error)
}

// RepositoryFactory crea instancias de repositorios.
//
// Uso:
//
//	factory := repository.NewPostgresFactory(db)
//	tradeRepo := factory.TradeRepository()
//	execRepo := factory.ExecutionRepository()
type RepositoryFactory interface {
	TradeRepository() TradeRepository
	ExecutionRepository() ExecutionRepository
	DedupeRepository() DedupeRepository
	CloseRepository() CloseRepository
	CorrelationService() CorrelationService
	SymbolRepository() SymbolRepository // NEW i3
	SymbolSpecRepository() SymbolSpecRepository
	SymbolQuoteRepository() SymbolQuoteRepository
	RiskPolicyRepository() RiskPolicyRepository
}
