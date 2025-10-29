package repository

import (
	"context"
	"fmt"

	"github.com/xKoRx/echo/sdk/domain"
)

// correlationService implementa domain.CorrelationService.
//
// Encapsula la lógica de correlación trade_id ↔ tickets usando
// los repositorios subyacentes.
type correlationService struct {
	executionRepo domain.ExecutionRepository
	dedupeRepo    domain.DedupeRepository
	closeRepo     domain.CloseRepository
}

// NewCorrelationService crea un nuevo servicio de correlación.
func NewCorrelationService(
	executionRepo domain.ExecutionRepository,
	dedupeRepo domain.DedupeRepository,
	closeRepo domain.CloseRepository,
) domain.CorrelationService {
	return &correlationService{
		executionRepo: executionRepo,
		dedupeRepo:    dedupeRepo,
		closeRepo:     closeRepo,
	}
}

// GetTicketsByTrade obtiene el mapa de slave_account_id → ticket para un trade.
//
// Solo retorna ejecuciones exitosas (success=true, ticket!=0).
//
// Uso en Core para CloseOrder:
//
//	tickets, err := correlationSvc.GetTicketsByTrade(ctx, tradeID)
//	ticket := tickets["67890"]  // ticket del slave 67890
func (s *correlationService) GetTicketsByTrade(ctx context.Context, tradeID string) (map[string]int32, error) {
	// Obtener todas las ejecuciones del trade
	executions, err := s.executionRepo.GetByTradeID(ctx, tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get executions for trade %s: %w", tradeID, err)
	}

	// Construir mapa de slave_account_id → ticket
	// Solo incluir ejecuciones exitosas con ticket válido
	tickets := make(map[string]int32)
	for _, exec := range executions {
		if exec.Success && exec.SlaveTicket != 0 {
			// Si hay múltiples ejecuciones del mismo slave, quedarse con la última
			// (aunque en i1 deberíamos tener solo una ejecución por slave)
			tickets[exec.SlaveAccountID] = exec.SlaveTicket
		}
	}

	return tickets, nil
}

// GetTicketForSlave obtiene el ticket de un trade en un slave específico.
//
// Retorna 0 si no existe o si falló la ejecución.
//
// Uso en Core para CloseOrder con target específico:
//
//	ticket, err := correlationSvc.GetTicketForSlave(ctx, tradeID, "67890")
//	if ticket == 0 {
//	    // No hay ejecución exitosa para este slave
//	}
func (s *correlationService) GetTicketForSlave(ctx context.Context, tradeID, slaveAccountID string) (int32, error) {
	ticket, err := s.executionRepo.GetTicketByTradeAndSlave(ctx, tradeID, slaveAccountID)
	if err != nil {
		return 0, fmt.Errorf("failed to get ticket for trade %s, slave %s: %w", tradeID, slaveAccountID, err)
	}
	return ticket, nil
}

// RecordExecution registra una ejecución (llamado tras recibir ExecutionResult).
//
// Flujo:
//  1. Crear entrada en executions
//  2. Actualizar dedupe status según resultado (FILLED si success, REJECTED si no)
//
// Uso en Core tras recibir ExecutionResult:
//
//	exec := &domain.Execution{...}
//	err := correlationSvc.RecordExecution(ctx, exec)
func (s *correlationService) RecordExecution(ctx context.Context, exec *domain.Execution) error {
	// 1. Crear ejecución
	if err := s.executionRepo.Create(ctx, exec); err != nil {
		return fmt.Errorf("failed to create execution: %w", err)
	}

	// 2. Determinar nuevo status de dedupe
	var newStatus domain.OrderStatus
	if exec.Success {
		newStatus = domain.OrderStatusFilled
	} else {
		newStatus = domain.OrderStatusRejected
	}

	// 3. Actualizar dedupe status
	// Usamos Upsert porque puede no existir aún (caso de ejecución antes de persistir intent)
	entry := &domain.DedupeEntry{
		TradeID: exec.TradeID,
		Status:  newStatus,
	}
	if err := s.dedupeRepo.Upsert(ctx, entry); err != nil {
		return fmt.Errorf("failed to upsert dedupe for trade %s: %w", exec.TradeID, err)
	}

	return nil
}

// RecordClose registra un cierre (llamado tras recibir CloseResult).
//
// Flujo:
//  1. Crear entrada en closes
//  2. NO actualizar dedupe status (mantener FILLED, no usar CANCELLED incorrectamente)
//
// Uso en Core tras recibir CloseResult:
//
//	close := &domain.Close{...}
//	err := correlationSvc.RecordClose(ctx, close)
//
// i1 FIX: CANCELLED es semánticamente incorrecto para cierres.
// CANCELLED = orden cancelada ANTES de ejecutarse (ej: pending order cancelada).
// Un cierre de posición FILLED NO debe cambiar el status de dedupe a CANCELLED.
// El status debe permanecer FILLED (la orden se ejecutó exitosamente y luego se cerró).
func (s *correlationService) RecordClose(ctx context.Context, close *domain.Close) error {
	// 1. Crear close
	if err := s.closeRepo.Create(ctx, close); err != nil {
		return fmt.Errorf("failed to create close: %w", err)
	}

	// 2. i1 FIX: NO actualizar dedupe status a CANCELLED
	// El cierre es un evento separado, no debe cambiar el status de la apertura.
	// La entrada en 'closes' ya registra el evento de cierre.
	// El status de dedupe debe permanecer FILLED (la orden se ejecutó exitosamente).
	//
	// TODO i2: Si necesitamos distinguir "abierta" vs "cerrada", agregar un nuevo status
	// como CLOSED o usar un flag separado. Por ahora, mantener FILLED.

	return nil
}

