package domain

import (
	"context"
	"time"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// DeliveryStatus representa el estado del comando en el journal.
type DeliveryStatus string

const (
	DeliveryStatusPending  DeliveryStatus = "pending"
	DeliveryStatusInflight DeliveryStatus = "inflight"
	DeliveryStatusAcked    DeliveryStatus = "acked"
	DeliveryStatusFailed   DeliveryStatus = "failed"
)

// DeliveryCommandType identifica el tipo de comando persistido en el journal.
type DeliveryCommandType string

const (
	DeliveryCommandTypeExecute DeliveryCommandType = "execute_order"
	DeliveryCommandTypeClose   DeliveryCommandType = "close_order"
)

// DeliveryJournalEntry modela una fila en delivery_journal.
type DeliveryJournalEntry struct {
	CommandID       string
	TradeID         string
	AgentID         string
	TargetAccountID string
	CommandType     DeliveryCommandType
	Payload         []byte
	Stage           pb.AckStage
	Status          DeliveryStatus
	Attempt         uint32
	NextRetryAt     time.Time
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// DeliveryRetryEvent modela una fila en delivery_retry_event (historial append-only).
type DeliveryRetryEvent struct {
	CommandID  string
	Stage      pb.AckStage
	Result     pb.AckResult
	Attempt    uint32
	Error      string
	DurationMs int64
	CreatedAt  time.Time
}

// DeliveryJournalRepository expone operaciones sobre delivery_journal.
type DeliveryJournalRepository interface {
	Insert(ctx context.Context, entry *DeliveryJournalEntry) error
	UpdateStatus(ctx context.Context, commandID string, stage pb.AckStage, status DeliveryStatus, attempt uint32, nextRetry time.Time, lastError string) error
	MarkAcked(ctx context.Context, commandID string, stage pb.AckStage) error
	GetDueEntries(ctx context.Context, status DeliveryStatus, before time.Time, limit int) ([]*DeliveryJournalEntry, error)
	GetByID(ctx context.Context, commandID string) (*DeliveryJournalEntry, error)
	AssignAgent(ctx context.Context, commandID, agentID string) error
}

// DeliveryRetryEventRepository expone operaciones sobre delivery_retry_event.
type DeliveryRetryEventRepository interface {
	InsertEvent(ctx context.Context, event *DeliveryRetryEvent) error
}
