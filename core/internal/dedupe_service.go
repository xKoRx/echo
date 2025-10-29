package internal

import (
	"context"
	"fmt"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// DedupeService servicio de deduplicación persistente (i1).
//
// Reemplaza DedupeStore in-memory de i0 con persistencia en PostgreSQL.
type DedupeService struct {
	repo domain.DedupeRepository
}

// NewDedupeService crea un nuevo servicio de deduplicación persistente.
func NewDedupeService(repo domain.DedupeRepository) *DedupeService {
	return &DedupeService{
		repo: repo,
	}
}

// Check verifica si un trade_id ya existe.
//
// Retorna:
//   - exists: true si el trade_id ya fue procesado
//   - status: el estado actual (o PENDING si no existe)
func (s *DedupeService) Check(ctx context.Context, tradeID string) (exists bool, status domain.OrderStatus, err error) {
	entry, err := s.repo.Get(ctx, tradeID)
	if err != nil {
		return false, "", fmt.Errorf("failed to check dedupe: %w", err)
	}

	if entry == nil {
		return false, domain.OrderStatusPending, nil
	}

	return true, entry.Status, nil
}

// Add agrega una nueva entrada de trade.
//
// Retorna error si el trade_id ya existe con status no terminal.
//
// En i1: si el trade_id ya existe y está en estado terminal (FILLED, REJECTED, CANCELLED),
// se permite re-agregar (caso de reinicio del Master EA con mismo trade_id tras TTL).
func (s *DedupeService) Add(ctx context.Context, tradeID string, status domain.OrderStatus) error {
	// Verificar si ya existe
	exists, currentStatus, err := s.Check(ctx, tradeID)
	if err != nil {
		return err
	}

	if exists {
		// Si ya existe y NO está en estado terminal, rechazar
		if !currentStatus.IsTerminal() {
			return &DedupeError{
				TradeID:        tradeID,
				ExistingStatus: s.domainStatusToProto(currentStatus),
				Message:        fmt.Sprintf("trade already exists with status %s", currentStatus),
			}
		}

		// Si está en estado terminal, permitir re-agregar (actualizar)
		// Esto cubre el caso de reinicio del Master EA tras TTL cleanup
	}

	// Upsert: crear o actualizar
	entry := &domain.DedupeEntry{
		TradeID: tradeID,
		Status:  status,
	}
	if err := s.repo.Upsert(ctx, entry); err != nil {
		return fmt.Errorf("failed to add dedupe entry: %w", err)
	}

	return nil
}

// UpdateStatus actualiza el status de un trade existente.
//
// No-op si el trade no existe (caso de ExecutionResult antes de TradeIntent).
func (s *DedupeService) UpdateStatus(ctx context.Context, tradeID string, newStatus domain.OrderStatus) error {
	// Verificar si existe
	exists, err := s.repo.Exists(ctx, tradeID)
	if err != nil {
		return fmt.Errorf("failed to check dedupe existence: %w", err)
	}

	if !exists {
		// No existe: crear entry nueva (caso de ExecutionResult antes de TradeIntent)
		entry := &domain.DedupeEntry{
			TradeID: tradeID,
			Status:  newStatus,
		}
		if err := s.repo.Upsert(ctx, entry); err != nil {
			return fmt.Errorf("failed to upsert dedupe entry: %w", err)
		}
		return nil
	}

	// Existe: actualizar status
	if err := s.repo.UpdateStatus(ctx, tradeID, newStatus); err != nil {
		return fmt.Errorf("failed to update dedupe status: %w", err)
	}

	return nil
}

// Get obtiene una entrada por trade_id.
//
// Retorna nil si no existe.
func (s *DedupeService) Get(ctx context.Context, tradeID string) (*domain.DedupeEntry, error) {
	entry, err := s.repo.Get(ctx, tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dedupe entry: %w", err)
	}
	return entry, nil
}

// Cleanup elimina entries antiguas (TTL expirado) llamando a la función SQL.
//
// Retorna el número de entries eliminadas.
func (s *DedupeService) Cleanup(ctx context.Context) (int, error) {
	deleted, err := s.repo.CleanupTTL(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup dedupe: %w", err)
	}
	return deleted, nil
}

// domainStatusToProto convierte domain.OrderStatus a pb.OrderStatus (para error).
func (s *DedupeService) domainStatusToProto(status domain.OrderStatus) pb.OrderStatus {
	switch status {
	case domain.OrderStatusPending:
		return pb.OrderStatus_ORDER_STATUS_PENDING
	case domain.OrderStatusFilled:
		return pb.OrderStatus_ORDER_STATUS_FILLED
	case domain.OrderStatusRejected:
		return pb.OrderStatus_ORDER_STATUS_REJECTED
	case domain.OrderStatusCancelled:
		return pb.OrderStatus_ORDER_STATUS_CANCELLED
	case domain.OrderStatusSent:
		// SENT no existe en proto v1, mapear a PENDING
		return pb.OrderStatus_ORDER_STATUS_PENDING
	default:
		return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

// Nota: DedupeError ya está definido en dedupe.go (i0).
// Lo reutilizamos para compatibilidad.

