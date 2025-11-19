package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

type postgresDeliveryJournalRepo struct {
	db *sql.DB
}

func (r *postgresDeliveryJournalRepo) Insert(ctx context.Context, entry *domain.DeliveryJournalEntry) error {
	query := `
		INSERT INTO echo.delivery_journal (
			command_id,
			trade_id,
			agent_id,
			target_account_id,
			command_type,
			payload,
			stage,
			status,
			attempt,
			next_retry_at,
			last_error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (command_id) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query,
		entry.CommandID,
		entry.TradeID,
		entry.AgentID,
		entry.TargetAccountID,
		string(entry.CommandType),
		entry.Payload,
		int32(entry.Stage),
		string(entry.Status),
		entry.Attempt,
		entry.NextRetryAt.UTC(),
		nullIfEmpty(entry.LastError),
	)
	if err != nil {
		return fmt.Errorf("insert delivery_journal: %w", err)
	}
	return nil
}

func (r *postgresDeliveryJournalRepo) UpdateStatus(ctx context.Context, commandID string, stage pb.AckStage, status domain.DeliveryStatus, attempt uint32, nextRetry time.Time, lastError string) error {
	query := `
		UPDATE echo.delivery_journal
		SET stage = $2,
		    status = $3,
		    attempt = $4,
		    next_retry_at = $5,
		    last_error = $6,
		    updated_at = NOW()
		WHERE command_id = $1
	`
	result, err := r.db.ExecContext(ctx, query,
		commandID,
		int32(stage),
		string(status),
		attempt,
		nextRetry.UTC(),
		nullIfEmpty(lastError),
	)
	if err != nil {
		return fmt.Errorf("update delivery_journal: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *postgresDeliveryJournalRepo) MarkAcked(ctx context.Context, commandID string, stage pb.AckStage) error {
	query := `
		UPDATE echo.delivery_journal
		SET stage = $2,
		    status = $3,
		    next_retry_at = NULL,
		    updated_at = NOW()
		WHERE command_id = $1
	`
	result, err := r.db.ExecContext(ctx, query,
		commandID,
		int32(stage),
		string(domain.DeliveryStatusAcked),
	)
	if err != nil {
		return fmt.Errorf("mark acked delivery_journal: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *postgresDeliveryJournalRepo) GetDueEntries(ctx context.Context, status domain.DeliveryStatus, before time.Time, limit int) ([]*domain.DeliveryJournalEntry, error) {
	query := `
		SELECT
			command_id,
			trade_id,
			agent_id,
			target_account_id,
			command_type,
			payload,
			stage,
			status,
			attempt,
			next_retry_at,
			COALESCE(last_error, ''),
			created_at,
			updated_at
		FROM echo.delivery_journal
		WHERE status = $1
		  AND next_retry_at IS NOT NULL
		  AND next_retry_at <= $2
		ORDER BY next_retry_at ASC
		LIMIT $3
	`
	rows, err := r.db.QueryContext(ctx, query, string(status), before.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("query due delivery_journal: %w", err)
	}
	defer rows.Close()

	var entries []*domain.DeliveryJournalEntry
	for rows.Next() {
		var entry domain.DeliveryJournalEntry
		var stage int32
		var nextRetry sql.NullTime
		if err := rows.Scan(
			&entry.CommandID,
			&entry.TradeID,
			&entry.AgentID,
			&entry.TargetAccountID,
			&entry.CommandType,
			&entry.Payload,
			&stage,
			&entry.Status,
			&entry.Attempt,
			&nextRetry,
			&entry.LastError,
			&entry.CreatedAt,
			&entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan delivery_journal: %w", err)
		}
		entry.Stage = pb.AckStage(stage)
		if nextRetry.Valid {
			entry.NextRetryAt = nextRetry.Time
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows delivery_journal: %w", err)
	}
	return entries, nil
}

func (r *postgresDeliveryJournalRepo) GetByID(ctx context.Context, commandID string) (*domain.DeliveryJournalEntry, error) {
	query := `
		SELECT
			command_id,
			trade_id,
			agent_id,
			target_account_id,
			command_type,
			payload,
			stage,
			status,
			attempt,
			next_retry_at,
			COALESCE(last_error, ''),
			created_at,
			updated_at
		FROM echo.delivery_journal
		WHERE command_id = $1
	`
	var entry domain.DeliveryJournalEntry
	var stage int32
	var nextRetry sql.NullTime
	err := r.db.QueryRowContext(ctx, query, commandID).Scan(
		&entry.CommandID,
		&entry.TradeID,
		&entry.AgentID,
		&entry.TargetAccountID,
		&entry.CommandType,
		&entry.Payload,
		&stage,
		&entry.Status,
		&entry.Attempt,
		&nextRetry,
		&entry.LastError,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery_journal: %w", err)
	}
	entry.Stage = pb.AckStage(stage)
	if nextRetry.Valid {
		entry.NextRetryAt = nextRetry.Time
	}
	return &entry, nil
}

func (r *postgresDeliveryJournalRepo) AssignAgent(ctx context.Context, commandID, agentID string) error {
	query := `
		UPDATE echo.delivery_journal
		SET agent_id = $2,
		    updated_at = NOW()
		WHERE command_id = $1
	`
	result, err := r.db.ExecContext(ctx, query, commandID, agentID)
	if err != nil {
		return fmt.Errorf("assign agent delivery_journal: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type postgresDeliveryRetryRepo struct {
	db *sql.DB
}

func (r *postgresDeliveryRetryRepo) InsertEvent(ctx context.Context, event *domain.DeliveryRetryEvent) error {
	query := `
		INSERT INTO echo.delivery_retry_event (
			command_id,
			stage,
			result,
			attempt,
			error,
			duration_ms
		) VALUES ($1,$2,$3,$4,$5,$6)
	`
	_, err := r.db.ExecContext(ctx, query,
		event.CommandID,
		int32(event.Stage),
		int32(event.Result),
		event.Attempt,
		nullIfEmpty(event.Error),
		event.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("insert delivery_retry_event: %w", err)
	}
	return nil
}

func nullIfEmpty(val string) interface{} {
	if val == "" {
		return nil
	}
	return val
}
