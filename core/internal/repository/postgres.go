// Package repository provee implementaciones de persistencia para Echo Core.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq" // Driver PostgreSQL
	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// PostgresFactory implementa domain.RepositoryFactory para PostgreSQL.
type PostgresFactory struct {
	db *sql.DB

	// Repositorios inicializados lazy
	tradeRepo       domain.TradeRepository
	executionRepo   domain.ExecutionRepository
	dedupeRepo      domain.DedupeRepository
	closeRepo       domain.CloseRepository
	correlationSvc  domain.CorrelationService
	symbolRepo      domain.SymbolRepository // NEW i3
	symbolSpecRepo  domain.SymbolSpecRepository
	symbolQuoteRepo domain.SymbolQuoteRepository
	riskPolicyRepo  domain.RiskPolicyRepository
}

// NewPostgresFactory crea un factory de repositorios PostgreSQL.
//
// Uso:
//
//	db, err := sql.Open("postgres", connStr)
//	factory := repository.NewPostgresFactory(db)
//	tradeRepo := factory.TradeRepository()
func NewPostgresFactory(db *sql.DB) *PostgresFactory {
	return &PostgresFactory{
		db: db,
	}
}

// TradeRepository retorna el repositorio de trades.
func (f *PostgresFactory) TradeRepository() domain.TradeRepository {
	if f.tradeRepo == nil {
		f.tradeRepo = &postgresTradeRepo{db: f.db}
	}
	return f.tradeRepo
}

// ExecutionRepository retorna el repositorio de executions.
func (f *PostgresFactory) ExecutionRepository() domain.ExecutionRepository {
	if f.executionRepo == nil {
		f.executionRepo = &postgresExecutionRepo{db: f.db}
	}
	return f.executionRepo
}

// DedupeRepository retorna el repositorio de dedupe.
func (f *PostgresFactory) DedupeRepository() domain.DedupeRepository {
	if f.dedupeRepo == nil {
		f.dedupeRepo = &postgresDedupeRepo{db: f.db}
	}
	return f.dedupeRepo
}

// CloseRepository retorna el repositorio de closes.
func (f *PostgresFactory) CloseRepository() domain.CloseRepository {
	if f.closeRepo == nil {
		f.closeRepo = &postgresCloseRepo{db: f.db}
	}
	return f.closeRepo
}

// CorrelationService retorna el servicio de correlación.
func (f *PostgresFactory) CorrelationService() domain.CorrelationService {
	if f.correlationSvc == nil {
		f.correlationSvc = NewCorrelationService(
			f.ExecutionRepository(),
			f.DedupeRepository(),
			f.CloseRepository(),
		)
	}
	return f.correlationSvc
}

// SymbolRepository retorna el repositorio de símbolos (i3).
func (f *PostgresFactory) SymbolRepository() domain.SymbolRepository {
	if f.symbolRepo == nil {
		f.symbolRepo = &postgresSymbolRepo{db: f.db}
	}
	return f.symbolRepo
}

// SymbolSpecRepository retorna el repositorio de especificaciones de símbolos.
func (f *PostgresFactory) SymbolSpecRepository() domain.SymbolSpecRepository {
	if f.symbolSpecRepo == nil {
		f.symbolSpecRepo = &postgresSymbolSpecRepo{db: f.db}
	}
	return f.symbolSpecRepo
}

// SymbolQuoteRepository retorna el repositorio de snapshots de precios.
func (f *PostgresFactory) SymbolQuoteRepository() domain.SymbolQuoteRepository {
	if f.symbolQuoteRepo == nil {
		f.symbolQuoteRepo = &postgresSymbolQuoteRepo{db: f.db}
	}
	return f.symbolQuoteRepo
}

// RiskPolicyRepository retorna el repositorio de políticas de riesgo.
func (f *PostgresFactory) RiskPolicyRepository() domain.RiskPolicyRepository {
	if f.riskPolicyRepo == nil {
		f.riskPolicyRepo = &postgresRiskPolicyRepo{db: f.db}
	}
	return f.riskPolicyRepo
}

// ===========================================================================
// postgresTradeRepo
// ===========================================================================

type postgresTradeRepo struct {
	db *sql.DB
}

func (r *postgresTradeRepo) Create(ctx context.Context, trade *domain.Trade) error {
	query := `
		INSERT INTO echo.trades (
			trade_id, source_master_id, master_account_id, master_ticket,
			magic_number, symbol, side, lot_size, price,
			stop_loss, take_profit, comment,
			status, attempt, opened_at_ms
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		trade.TradeID,
		trade.SourceMasterID,
		trade.MasterAccountID,
		trade.MasterTicket,
		trade.MagicNumber,
		trade.Symbol,
		trade.Side,
		trade.LotSize,
		trade.Price,
		trade.StopLoss,
		trade.TakeProfit,
		trade.Comment,
		trade.Status,
		trade.Attempt,
		trade.OpenedAtMs,
	)
	if err != nil {
		return fmt.Errorf("failed to create trade: %w", err)
	}
	return nil
}

func (r *postgresTradeRepo) GetByID(ctx context.Context, tradeID string) (*domain.Trade, error) {
	query := `
		SELECT trade_id, source_master_id, master_account_id, master_ticket,
		       magic_number, symbol, side, lot_size, price,
		       stop_loss, take_profit, comment,
		       status, attempt, opened_at_ms, created_at, updated_at
		FROM echo.trades
		WHERE trade_id = $1
	`
	var trade domain.Trade
	err := r.db.QueryRowContext(ctx, query, tradeID).Scan(
		&trade.TradeID,
		&trade.SourceMasterID,
		&trade.MasterAccountID,
		&trade.MasterTicket,
		&trade.MagicNumber,
		&trade.Symbol,
		&trade.Side,
		&trade.LotSize,
		&trade.Price,
		&trade.StopLoss,
		&trade.TakeProfit,
		&trade.Comment,
		&trade.Status,
		&trade.Attempt,
		&trade.OpenedAtMs,
		&trade.CreatedAt,
		&trade.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trade: %w", err)
	}
	return &trade, nil
}

func (r *postgresTradeRepo) GetByMasterTicket(ctx context.Context, masterAccountID string, masterTicket int32) (*domain.Trade, error) {
	query := `
		SELECT trade_id, source_master_id, master_account_id, master_ticket,
		       magic_number, symbol, side, lot_size, price,
		       stop_loss, take_profit, comment,
		       status, attempt, opened_at_ms, created_at, updated_at
		FROM echo.trades
		WHERE master_account_id = $1 AND master_ticket = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var trade domain.Trade
	err := r.db.QueryRowContext(ctx, query, masterAccountID, masterTicket).Scan(
		&trade.TradeID,
		&trade.SourceMasterID,
		&trade.MasterAccountID,
		&trade.MasterTicket,
		&trade.MagicNumber,
		&trade.Symbol,
		&trade.Side,
		&trade.LotSize,
		&trade.Price,
		&trade.StopLoss,
		&trade.TakeProfit,
		&trade.Comment,
		&trade.Status,
		&trade.Attempt,
		&trade.OpenedAtMs,
		&trade.CreatedAt,
		&trade.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trade by master ticket: %w", err)
	}
	return &trade, nil
}

func (r *postgresTradeRepo) UpdateStatus(ctx context.Context, tradeID string, status domain.OrderStatus) error {
	query := `
		UPDATE echo.trades
		SET status = $1, updated_at = NOW()
		WHERE trade_id = $2
	`
	result, err := r.db.ExecContext(ctx, query, status, tradeID)
	if err != nil {
		return fmt.Errorf("failed to update trade status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("trade not found: %s", tradeID)
	}
	return nil
}

func (r *postgresTradeRepo) List(ctx context.Context, limit, offset int) ([]*domain.Trade, error) {
	query := `
		SELECT trade_id, source_master_id, master_account_id, master_ticket,
		       magic_number, symbol, side, lot_size, price,
		       stop_loss, take_profit, comment,
		       status, attempt, opened_at_ms, created_at, updated_at
		FROM echo.trades
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	return r.queryTrades(ctx, query, limit, offset)
}

func (r *postgresTradeRepo) ListByStatus(ctx context.Context, status domain.OrderStatus, limit, offset int) ([]*domain.Trade, error) {
	query := `
		SELECT trade_id, source_master_id, master_account_id, master_ticket,
		       magic_number, symbol, side, lot_size, price,
		       stop_loss, take_profit, comment,
		       status, attempt, opened_at_ms, created_at, updated_at
		FROM echo.trades
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	return r.queryTrades(ctx, query, status, limit, offset)
}

func (r *postgresTradeRepo) queryTrades(ctx context.Context, query string, args ...interface{}) ([]*domain.Trade, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trades: %w", err)
	}
	defer rows.Close()

	var trades []*domain.Trade
	for rows.Next() {
		var trade domain.Trade
		err := rows.Scan(
			&trade.TradeID,
			&trade.SourceMasterID,
			&trade.MasterAccountID,
			&trade.MasterTicket,
			&trade.MagicNumber,
			&trade.Symbol,
			&trade.Side,
			&trade.LotSize,
			&trade.Price,
			&trade.StopLoss,
			&trade.TakeProfit,
			&trade.Comment,
			&trade.Status,
			&trade.Attempt,
			&trade.OpenedAtMs,
			&trade.CreatedAt,
			&trade.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trade: %w", err)
		}
		trades = append(trades, &trade)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return trades, nil
}

// ===========================================================================
// postgresExecutionRepo
// ===========================================================================

type postgresExecutionRepo struct {
	db *sql.DB
}

func (r *postgresExecutionRepo) Create(ctx context.Context, exec *domain.Execution) error {
	// Serializar timestamps_ms a JSONB
	timestampsJSON, err := json.Marshal(exec.TimestampsMs)
	if err != nil {
		return fmt.Errorf("failed to marshal timestamps: %w", err)
	}

	query := `
		INSERT INTO echo.executions (
			execution_id, trade_id, slave_account_id, agent_id,
			slave_ticket, executed_price, success, error_code, error_message,
			timestamps_ms
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`
	_, err = r.db.ExecContext(ctx, query,
		exec.ExecutionID,
		exec.TradeID,
		exec.SlaveAccountID,
		exec.AgentID,
		exec.SlaveTicket,
		exec.ExecutedPrice,
		exec.Success,
		exec.ErrorCode,
		exec.ErrorMessage,
		timestampsJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to create execution: %w", err)
	}
	return nil
}

func (r *postgresExecutionRepo) GetByID(ctx context.Context, executionID string) (*domain.Execution, error) {
	query := `
		SELECT execution_id, trade_id, slave_account_id, agent_id,
		       slave_ticket, executed_price, success, error_code, error_message,
		       timestamps_ms, created_at
		FROM echo.executions
		WHERE execution_id = $1
	`
	var exec domain.Execution
	var timestampsJSON []byte
	err := r.db.QueryRowContext(ctx, query, executionID).Scan(
		&exec.ExecutionID,
		&exec.TradeID,
		&exec.SlaveAccountID,
		&exec.AgentID,
		&exec.SlaveTicket,
		&exec.ExecutedPrice,
		&exec.Success,
		&exec.ErrorCode,
		&exec.ErrorMessage,
		&timestampsJSON,
		&exec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	// Deserializar timestamps
	if err := json.Unmarshal(timestampsJSON, &exec.TimestampsMs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal timestamps: %w", err)
	}

	return &exec, nil
}

func (r *postgresExecutionRepo) GetByTradeID(ctx context.Context, tradeID string) ([]*domain.Execution, error) {
	query := `
		SELECT execution_id, trade_id, slave_account_id, agent_id,
		       slave_ticket, executed_price, success, error_code, error_message,
		       timestamps_ms, created_at
		FROM echo.executions
		WHERE trade_id = $1
		ORDER BY created_at ASC
	`
	return r.queryExecutions(ctx, query, tradeID)
}

func (r *postgresExecutionRepo) GetByTradeAndSlave(ctx context.Context, tradeID, slaveAccountID string) (*domain.Execution, error) {
	query := `
		SELECT execution_id, trade_id, slave_account_id, agent_id,
		       slave_ticket, executed_price, success, error_code, error_message,
		       timestamps_ms, created_at
		FROM echo.executions
		WHERE trade_id = $1 AND slave_account_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	execs, err := r.queryExecutions(ctx, query, tradeID, slaveAccountID)
	if err != nil {
		return nil, err
	}
	if len(execs) == 0 {
		return nil, nil
	}
	return execs[0], nil
}

func (r *postgresExecutionRepo) GetTicketByTradeAndSlave(ctx context.Context, tradeID, slaveAccountID string) (int32, error) {
	query := `
		SELECT slave_ticket
		FROM echo.executions
		WHERE trade_id = $1 AND slave_account_id = $2 AND success = true AND slave_ticket != 0
		ORDER BY created_at DESC
		LIMIT 1
	`
	var ticket int32
	err := r.db.QueryRowContext(ctx, query, tradeID, slaveAccountID).Scan(&ticket)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get ticket: %w", err)
	}
	return ticket, nil
}

func (r *postgresExecutionRepo) List(ctx context.Context, limit, offset int) ([]*domain.Execution, error) {
	query := `
		SELECT execution_id, trade_id, slave_account_id, agent_id,
		       slave_ticket, executed_price, success, error_code, error_message,
		       timestamps_ms, created_at
		FROM echo.executions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	return r.queryExecutions(ctx, query, limit, offset)
}

func (r *postgresExecutionRepo) ListBySuccess(ctx context.Context, success bool, limit, offset int) ([]*domain.Execution, error) {
	query := `
		SELECT execution_id, trade_id, slave_account_id, agent_id,
		       slave_ticket, executed_price, success, error_code, error_message,
		       timestamps_ms, created_at
		FROM echo.executions
		WHERE success = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	return r.queryExecutions(ctx, query, success, limit, offset)
}

func (r *postgresExecutionRepo) queryExecutions(ctx context.Context, query string, args ...interface{}) ([]*domain.Execution, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer rows.Close()

	var execs []*domain.Execution
	for rows.Next() {
		var exec domain.Execution
		var timestampsJSON []byte
		err := rows.Scan(
			&exec.ExecutionID,
			&exec.TradeID,
			&exec.SlaveAccountID,
			&exec.AgentID,
			&exec.SlaveTicket,
			&exec.ExecutedPrice,
			&exec.Success,
			&exec.ErrorCode,
			&exec.ErrorMessage,
			&timestampsJSON,
			&exec.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan execution: %w", err)
		}

		// Deserializar timestamps
		if err := json.Unmarshal(timestampsJSON, &exec.TimestampsMs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal timestamps: %w", err)
		}

		execs = append(execs, &exec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return execs, nil
}

// ===========================================================================
// postgresDedupeRepo
// ===========================================================================

type postgresDedupeRepo struct {
	db *sql.DB
}

func (r *postgresDedupeRepo) Upsert(ctx context.Context, entry *domain.DedupeEntry) error {
	query := `
		INSERT INTO echo.dedupe (trade_id, status)
		VALUES ($1, $2)
		ON CONFLICT (trade_id) DO UPDATE
		SET status = EXCLUDED.status, updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, query, entry.TradeID, entry.Status)
	if err != nil {
		return fmt.Errorf("failed to upsert dedupe entry: %w", err)
	}
	return nil
}

func (r *postgresDedupeRepo) Get(ctx context.Context, tradeID string) (*domain.DedupeEntry, error) {
	query := `
		SELECT trade_id, status, created_at, updated_at
		FROM echo.dedupe
		WHERE trade_id = $1
	`
	var entry domain.DedupeEntry
	err := r.db.QueryRowContext(ctx, query, tradeID).Scan(
		&entry.TradeID,
		&entry.Status,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dedupe entry: %w", err)
	}
	return &entry, nil
}

func (r *postgresDedupeRepo) Exists(ctx context.Context, tradeID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM echo.dedupe WHERE trade_id = $1)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, tradeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check dedupe existence: %w", err)
	}
	return exists, nil
}

func (r *postgresDedupeRepo) UpdateStatus(ctx context.Context, tradeID string, status domain.OrderStatus) error {
	query := `
		UPDATE echo.dedupe
		SET status = $1, updated_at = NOW()
		WHERE trade_id = $2
	`
	result, err := r.db.ExecContext(ctx, query, status, tradeID)
	if err != nil {
		return fmt.Errorf("failed to update dedupe status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("dedupe entry not found: %s", tradeID)
	}
	return nil
}

func (r *postgresDedupeRepo) CleanupTTL(ctx context.Context) (int, error) {
	// Llamar a la función SQL que limpia entries antiguos
	query := `SELECT echo.cleanup_dedupe_ttl()`
	var deleted int
	err := r.db.QueryRowContext(ctx, query).Scan(&deleted)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup dedupe TTL: %w", err)
	}
	return deleted, nil
}

// ===========================================================================
// postgresCloseRepo
// ===========================================================================

type postgresCloseRepo struct {
	db *sql.DB
}

func (r *postgresCloseRepo) Create(ctx context.Context, close *domain.Close) error {
	query := `
		INSERT INTO echo.closes (
			close_id, trade_id, slave_account_id, slave_ticket,
			close_price, success, error_code, error_message, closed_at_ms
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		close.CloseID,
		close.TradeID,
		close.SlaveAccountID,
		close.SlaveTicket,
		close.ClosePrice,
		close.Success,
		close.ErrorCode,
		close.ErrorMessage,
		close.ClosedAtMs,
	)
	if err != nil {
		return fmt.Errorf("failed to create close: %w", err)
	}
	return nil
}

func (r *postgresCloseRepo) GetByID(ctx context.Context, closeID string) (*domain.Close, error) {
	query := `
		SELECT close_id, trade_id, slave_account_id, slave_ticket,
		       close_price, success, error_code, error_message, closed_at_ms, created_at
		FROM echo.closes
		WHERE close_id = $1
	`
	var close domain.Close
	err := r.db.QueryRowContext(ctx, query, closeID).Scan(
		&close.CloseID,
		&close.TradeID,
		&close.SlaveAccountID,
		&close.SlaveTicket,
		&close.ClosePrice,
		&close.Success,
		&close.ErrorCode,
		&close.ErrorMessage,
		&close.ClosedAtMs,
		&close.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get close: %w", err)
	}
	return &close, nil
}

func (r *postgresCloseRepo) GetByTradeID(ctx context.Context, tradeID string) ([]*domain.Close, error) {
	query := `
		SELECT close_id, trade_id, slave_account_id, slave_ticket,
		       close_price, success, error_code, error_message, closed_at_ms, created_at
		FROM echo.closes
		WHERE trade_id = $1
		ORDER BY created_at ASC
	`
	return r.queryCloses(ctx, query, tradeID)
}

func (r *postgresCloseRepo) GetByTradeAndSlave(ctx context.Context, tradeID, slaveAccountID string) (*domain.Close, error) {
	query := `
		SELECT close_id, trade_id, slave_account_id, slave_ticket,
		       close_price, success, error_code, error_message, closed_at_ms, created_at
		FROM echo.closes
		WHERE trade_id = $1 AND slave_account_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	closes, err := r.queryCloses(ctx, query, tradeID, slaveAccountID)
	if err != nil {
		return nil, err
	}
	if len(closes) == 0 {
		return nil, nil
	}
	return closes[0], nil
}

func (r *postgresCloseRepo) List(ctx context.Context, limit, offset int) ([]*domain.Close, error) {
	query := `
		SELECT close_id, trade_id, slave_account_id, slave_ticket,
		       close_price, success, error_code, error_message, closed_at_ms, created_at
		FROM echo.closes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	return r.queryCloses(ctx, query, limit, offset)
}

func (r *postgresCloseRepo) queryCloses(ctx context.Context, query string, args ...interface{}) ([]*domain.Close, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query closes: %w", err)
	}
	defer rows.Close()

	var closes []*domain.Close
	for rows.Next() {
		var close domain.Close
		err := rows.Scan(
			&close.CloseID,
			&close.TradeID,
			&close.SlaveAccountID,
			&close.SlaveTicket,
			&close.ClosePrice,
			&close.Success,
			&close.ErrorCode,
			&close.ErrorMessage,
			&close.ClosedAtMs,
			&close.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan close: %w", err)
		}
		closes = append(closes, &close)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return closes, nil
}

// ===========================================================================
// postgresSymbolRepo (i3)
// ===========================================================================

type postgresSymbolRepo struct {
	db *sql.DB
}

func (r *postgresSymbolRepo) UpsertAccountMapping(ctx context.Context, accountID string, mappings []*domain.SymbolMapping, reportedAtMs int64) error {
	// Usar transacción para asegurar atomicidad del reemplazo completo
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Upsert cada mapping con idempotencia temporal
	for _, m := range mappings {
		query := `
			INSERT INTO echo.account_symbol_map (
				account_id, canonical_symbol, broker_symbol,
				digits, point, tick_size, min_lot, max_lot, lot_step, stop_level,
				contract_size, reported_at_ms, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
			ON CONFLICT (account_id, canonical_symbol)
			DO UPDATE SET
				broker_symbol = EXCLUDED.broker_symbol,
				digits = EXCLUDED.digits,
				point = EXCLUDED.point,
				tick_size = EXCLUDED.tick_size,
				min_lot = EXCLUDED.min_lot,
				max_lot = EXCLUDED.max_lot,
				lot_step = EXCLUDED.lot_step,
				stop_level = EXCLUDED.stop_level,
				contract_size = EXCLUDED.contract_size,
				reported_at_ms = EXCLUDED.reported_at_ms,
				updated_at = NOW()
			WHERE EXCLUDED.reported_at_ms >= echo.account_symbol_map.reported_at_ms
		`
		_, err := tx.ExecContext(ctx, query,
			accountID,
			m.CanonicalSymbol,
			m.BrokerSymbol,
			m.Digits,
			m.Point,
			m.TickSize,
			m.MinLot,
			m.MaxLot,
			m.LotStep,
			m.StopLevel,
			m.ContractSize,
			reportedAtMs,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert symbol mapping: %w", err)
		}
	}

	// Commit transacción
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ==========================================================================
// postgresSymbolSpecRepo
// ==========================================================================

type postgresSymbolSpecRepo struct {
	db *sql.DB
}

func (r *postgresSymbolSpecRepo) UpsertSpecifications(ctx context.Context, accountID string, specs []*pb.SymbolSpecification, reportedAtMs int64) error {
	if len(specs) == 0 {
		return nil
	}

	query := `
		INSERT INTO echo.account_symbol_spec (
			account_id, canonical_symbol, broker_symbol, payload, reported_at_ms, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, NOW()
		)
		ON CONFLICT (account_id, canonical_symbol)
		DO UPDATE SET
			broker_symbol   = EXCLUDED.broker_symbol,
			payload         = EXCLUDED.payload,
			reported_at_ms  = EXCLUDED.reported_at_ms,
			updated_at      = NOW()
		WHERE EXCLUDED.reported_at_ms >= echo.account_symbol_spec.reported_at_ms;
	`

	for _, spec := range specs {
		if spec == nil {
			continue
		}

		payload, err := protojson.Marshal(spec)
		if err != nil {
			return fmt.Errorf("failed to marshal symbol specification: %w", err)
		}

		if _, err := r.db.ExecContext(ctx, query, accountID, spec.CanonicalSymbol, spec.BrokerSymbol, payload, reportedAtMs); err != nil {
			return fmt.Errorf("failed to upsert symbol specification (canonical=%s): %w", spec.CanonicalSymbol, err)
		}
	}

	return nil
}

func (r *postgresSymbolSpecRepo) GetSpecifications(ctx context.Context, accountID string) (map[string]*pb.SymbolSpecification, error) {
	query := `
		SELECT canonical_symbol, payload
		FROM echo.account_symbol_spec
		WHERE account_id = $1
	`

	rows, err := r.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query symbol specifications: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*pb.SymbolSpecification)
	for rows.Next() {
		var canonical string
		var payload []byte
		if err := rows.Scan(&canonical, &payload); err != nil {
			return nil, fmt.Errorf("failed to scan symbol specification: %w", err)
		}

		var spec pb.SymbolSpecification
		if err := protojson.Unmarshal(payload, &spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal symbol specification: %w", err)
		}

		result[canonical] = &spec
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error while reading symbol specifications: %w", err)
	}

	return result, nil
}

// ==========================================================================
// postgresSymbolQuoteRepo
// ==========================================================================

type postgresSymbolQuoteRepo struct {
	db *sql.DB
}

// ==========================================================================
// postgresRiskPolicyRepo
// ==========================================================================

type postgresRiskPolicyRepo struct {
	db *sql.DB
}

func (r *postgresRiskPolicyRepo) Get(ctx context.Context, accountID, strategyID string) (*domain.RiskPolicy, error) {
	query := `
		SELECT risk_type, lot_size, version, updated_at, valid_until
		FROM echo.account_strategy_risk_policy
		WHERE account_id = $1 AND strategy_id = $2
	`

	row := r.db.QueryRowContext(ctx, query, accountID, strategyID)

	var (
		riskType string
		lotSize sql.NullFloat64
		version sql.NullInt64
		updatedAt time.Time
		validUntil sql.NullTime
	)

	if err := row.Scan(&riskType, &lotSize, &version, &updatedAt, &validUntil); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get risk policy: %w", err)
	}

	policy := &domain.RiskPolicy{
		AccountID:  accountID,
		StrategyID: strategyID,
		Type:       domain.RiskPolicyType(riskType),
		Version:    version.Int64,
		UpdatedAt:  updatedAt,
	}

	if validUntil.Valid {
		policy.ValidUntil = &validUntil.Time
	}

	switch policy.Type {
	case domain.RiskPolicyTypeFixedLot:
		if !lotSize.Valid {
			return nil, fmt.Errorf("risk policy FIXED_LOT missing lot_size (account=%s, strategy=%s)", accountID, strategyID)
		}
		policy.FixedLot = &domain.FixedLotConfig{LotSize: lotSize.Float64}
	default:
		return nil, fmt.Errorf("unsupported risk policy type: %s", riskType)
	}

	return policy, nil
}

func (r *postgresSymbolQuoteRepo) InsertSnapshot(ctx context.Context, snapshot *pb.SymbolQuoteSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	query := `
		INSERT INTO echo.symbol_quote_latest (
			account_id, canonical_symbol, broker_symbol,
			bid, ask, spread_points, timestamp_ms, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NOW()
		)
		ON CONFLICT (account_id, canonical_symbol)
		DO UPDATE SET
			broker_symbol  = EXCLUDED.broker_symbol,
			bid            = EXCLUDED.bid,
			ask            = EXCLUDED.ask,
			spread_points  = EXCLUDED.spread_points,
			timestamp_ms   = EXCLUDED.timestamp_ms,
			updated_at     = NOW()
		WHERE EXCLUDED.timestamp_ms >= echo.symbol_quote_latest.timestamp_ms;
	`

	if _, err := r.db.ExecContext(ctx, query,
		snapshot.AccountId,
		snapshot.CanonicalSymbol,
		snapshot.BrokerSymbol,
		snapshot.Bid,
		snapshot.Ask,
		snapshot.SpreadPoints,
		snapshot.TimestampMs,
	); err != nil {
		return fmt.Errorf("failed to upsert symbol quote snapshot: %w", err)
	}

	return nil
}

func (r *postgresSymbolQuoteRepo) GetLatestSnapshot(ctx context.Context, accountID, canonicalSymbol string) (*pb.SymbolQuoteSnapshot, error) {
	query := `
		SELECT broker_symbol, bid, ask, spread_points, timestamp_ms
		FROM echo.symbol_quote_latest
		WHERE account_id = $1 AND canonical_symbol = $2
	`

	var snapshot pb.SymbolQuoteSnapshot
	var broker string
	var bid, ask, spread float64
	var timestamp int64
	row := r.db.QueryRowContext(ctx, query, accountID, canonicalSymbol)
	if err := row.Scan(&broker, &bid, &ask, &spread, &timestamp); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest snapshot: %w", err)
	}

	snapshot.AccountId = accountID
	snapshot.CanonicalSymbol = canonicalSymbol
	snapshot.BrokerSymbol = broker
	snapshot.Bid = bid
	snapshot.Ask = ask
	snapshot.SpreadPoints = spread
	snapshot.TimestampMs = timestamp

	return &snapshot, nil
}

func (r *postgresSymbolRepo) GetAccountMapping(ctx context.Context, accountID string) (map[string]*domain.AccountSymbolInfo, error) {
	query := `
		SELECT canonical_symbol, broker_symbol,
		       digits, point, tick_size, min_lot, max_lot, lot_step, stop_level, contract_size
		FROM echo.account_symbol_map
		WHERE account_id = $1
	`
	rows, err := r.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query account mapping: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*domain.AccountSymbolInfo)
	for rows.Next() {
		var canonical, broker string
		var digits, stopLevel int32
		var point, tickSize, minLot, maxLot, lotStep float64
		var contractSize sql.NullFloat64

		err := rows.Scan(
			&canonical,
			&broker,
			&digits,
			&point,
			&tickSize,
			&minLot,
			&maxLot,
			&lotStep,
			&stopLevel,
			&contractSize,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan symbol mapping: %w", err)
		}

		var contractSizePtr *float64
		if contractSize.Valid {
			contractSizePtr = &contractSize.Float64
		}

		result[canonical] = &domain.AccountSymbolInfo{
			BrokerSymbol:    broker,
			CanonicalSymbol: canonical,
			Digits:          digits,
			Point:           point,
			TickSize:        tickSize,
			MinLot:          minLot,
			MaxLot:          maxLot,
			LotStep:         lotStep,
			StopLevel:       stopLevel,
			ContractSize:    contractSizePtr,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
}

func (r *postgresSymbolRepo) InvalidateAccount(ctx context.Context, accountID string) error {
	query := `DELETE FROM echo.account_symbol_map WHERE account_id = $1`
	_, err := r.db.ExecContext(ctx, query, accountID)
	if err != nil {
		return fmt.Errorf("failed to invalidate account mappings: %w", err)
	}
	return nil
}
