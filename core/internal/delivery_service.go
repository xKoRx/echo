package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	deliveryBatchSize       = 128
	pendingAgentPlaceholder = "$pending"
)

// DeliveryService implementa el journal y reintentos Core↔Agent (i17).
type DeliveryService struct {
	core      *Core
	cfg       DeliveryConfig
	cfgMu     sync.RWMutex
	repo      domain.DeliveryJournalRepository
	retryRepo domain.DeliveryRetryEventRepository

	telemetry *telemetry.Client

	ctx    context.Context
	cancel context.CancelFunc

	reconciler *deliveryReconciler
}

func NewDeliveryService(core *Core) *DeliveryService {
	ctx, cancel := context.WithCancel(core.ctx)
	svc := &DeliveryService{
		core:      core,
		cfg:       core.config.Delivery,
		repo:      core.repoFactory.DeliveryJournalRepository(),
		retryRepo: core.repoFactory.DeliveryRetryEventRepository(),
		telemetry: core.telemetry,
		ctx:       ctx,
		cancel:    cancel,
	}
	svc.reconciler = newDeliveryReconciler(svc)
	return svc
}

func (s *DeliveryService) Start() error {
	if s.reconciler != nil {
		return s.reconciler.Start()
	}
	return nil
}

func (s *DeliveryService) Stop() {
	s.cancel()
	if s.reconciler != nil {
		s.reconciler.Stop()
	}
}

func (s *DeliveryService) getConfig() DeliveryConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

func (s *DeliveryService) UpdateConfig(cfg DeliveryConfig) {
	s.cfgMu.Lock()
	s.cfg = cfg
	s.cfgMu.Unlock()
	if s.reconciler != nil {
		s.reconciler.UpdateInterval(cfg.ReconcilerInterval)
	}
}

func (s *DeliveryService) ScheduleExecuteOrder(ctx context.Context, agentID string, order *pb.ExecuteOrder) error {
	if order == nil {
		return fmt.Errorf("execute order nil")
	}
	msg := &pb.CoreMessage{
		Payload: &pb.CoreMessage_ExecuteOrder{
			ExecuteOrder: order,
		},
	}
	return s.schedule(ctx, agentID, order.TargetAccountId, order.TradeId, domain.DeliveryCommandTypeExecute, msg)
}

func (s *DeliveryService) ScheduleCloseOrder(ctx context.Context, agentID string, order *pb.CloseOrder) error {
	if order == nil {
		return fmt.Errorf("close order nil")
	}
	msg := &pb.CoreMessage{
		Payload: &pb.CoreMessage_CloseOrder{
			CloseOrder: order,
		},
	}
	return s.schedule(ctx, agentID, order.TargetAccountId, order.TradeId, domain.DeliveryCommandTypeClose, msg)
}

func (s *DeliveryService) HandleCommandAck(ctx context.Context, agentID string, ack *pb.CommandAck) error {
	if ack == nil {
		return fmt.Errorf("command ack nil")
	}

	entry, err := s.repo.GetByID(ctx, ack.CommandId)
	if err != nil {
		return fmt.Errorf("get journal entry: %w", err)
	}
	if entry == nil {
		s.telemetry.Warn(s.ctx, "delivery ack for unknown command",
			semconv.Echo.CommandID.String(ack.CommandId),
			attribute.String("agent_id", agentID),
		)
		return nil
	}

	stage := ack.GetStage()
	status := domain.DeliveryStatusInflight
	cfg := s.getConfig()
	nextRetry := time.Now().Add(cfg.AckTimeout)
	lastError := ""

	if ack.Result == pb.AckResult_ACK_RESULT_FAILED {
		status = domain.DeliveryStatusFailed
		nextRetry = time.Time{}
		lastError = ack.ErrorCode.String()
	} else if stage == pb.AckStage_ACK_STAGE_EA_CONFIRMED {
		status = domain.DeliveryStatusAcked
		nextRetry = time.Time{}
	}

	if err := s.withJournalRetry(ctx, "delivery_journal.update_ack", func(dbCtx context.Context) error {
		return s.repo.UpdateStatus(dbCtx, ack.CommandId, stage, status, entry.Attempt, nextRetry, lastError)
	}); err != nil {
		return fmt.Errorf("update delivery ack: %w", err)
	}

	s.telemetry.Info(s.ctx, "delivery ack processed",
		semconv.Echo.CommandID.String(ack.CommandId),
		attribute.String("agent_id", agentID),
		attribute.String("stage", stage.String()),
		attribute.String("result", ack.Result.String()),
	)
	if status == domain.DeliveryStatusAcked {
		s.recordPendingAge(entry, stage)
	}
	return nil
}

func (s *DeliveryService) schedule(ctx context.Context, agentID, accountID, tradeID string, cmdType domain.DeliveryCommandType, msg *pb.CoreMessage) error {
	payload, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal delivery payload: %w", err)
	}
	if agentID == "" {
		agentID = pendingAgentPlaceholder
	}

	entry := &domain.DeliveryJournalEntry{
		CommandID:       commandIDFromMessage(msg),
		TradeID:         tradeID,
		AgentID:         agentID,
		TargetAccountID: accountID,
		CommandType:     cmdType,
		Payload:         payload,
		Stage:           pb.AckStage_ACK_STAGE_UNSPECIFIED,
		Status:          domain.DeliveryStatusPending,
		Attempt:         0,
		NextRetryAt:     time.Now(),
	}

	if entry.CommandID == "" {
		entry.CommandID = utils.GenerateUUIDv7()
	}

	spanCtx, span := s.telemetry.StartSpan(ctx, "core.delivery.journal")
	cfg := s.getConfig()
	span.SetAttributes(
		semconv.Echo.CommandID.String(entry.CommandID),
		semconv.Echo.TradeID.String(entry.TradeID),
		semconv.Echo.AccountID.String(entry.TargetAccountID),
		attribute.String("agent_id", entry.AgentID),
		attribute.String("stage", entry.Stage.String()),
		attribute.Int("attempt", int(entry.Attempt)),
		attribute.Int64("ack_timeout_ms", cfg.AckTimeout.Milliseconds()),
	)
	defer span.End()

	if err := s.withJournalRetry(spanCtx, "delivery_journal.insert", func(dbCtx context.Context) error {
		return s.repo.Insert(dbCtx, entry)
	}); err != nil {
		s.telemetry.RecordError(spanCtx, err)
		return fmt.Errorf("insert delivery journal: %w", err)
	}

	return s.dispatch(spanCtx, entry, msg)
}

func (s *DeliveryService) dispatch(ctx context.Context, entry *domain.DeliveryJournalEntry, msg *pb.CoreMessage) error {
	spanCtx, span := s.telemetry.StartSpan(ctx, "core.delivery.retry")
	cfg := s.getConfig()
	backoff := s.nextBackoff(cfg, entry.Attempt+1)
	span.SetAttributes(
		semconv.Echo.CommandID.String(entry.CommandID),
		semconv.Echo.AccountID.String(entry.TargetAccountID),
		attribute.String("agent_id", entry.AgentID),
		attribute.String("stage", entry.Stage.String()),
		attribute.Int("attempt", int(entry.Attempt+1)),
		attribute.Int64("ack_timeout_ms", cfg.AckTimeout.Milliseconds()),
		attribute.Int64("backoff_ms", backoff.Milliseconds()),
		attribute.Bool("compat_mode", !s.core.agentSupportsLossless(entry.AgentID)),
	)
	defer span.End()

	if err := s.ensureAgentAssignment(spanCtx, entry); err != nil {
		s.telemetry.Warn(s.ctx, "delivery pending waiting owner",
			semconv.Echo.CommandID.String(entry.CommandID),
			semconv.Echo.AccountID.String(entry.TargetAccountID),
			attribute.String("error", err.Error()),
		)
		s.telemetry.RecordError(spanCtx, err)
		return s.markPending(spanCtx, entry, err)
	}

	if !s.core.agentSupportsLossless(entry.AgentID) {
		if err := s.core.sendMessageToAgent(spanCtx, entry.AgentID, msg); err != nil {
			s.telemetry.RecordError(spanCtx, err)
			return s.markPending(spanCtx, entry, err)
		}
		if err := s.withJournalRetry(spanCtx, "delivery_journal.mark_compat_ack", func(dbCtx context.Context) error {
			return s.repo.MarkAcked(dbCtx, entry.CommandID, pb.AckStage_ACK_STAGE_CORE_ACCEPTED)
		}); err != nil {
			s.telemetry.RecordError(spanCtx, err)
			return fmt.Errorf("mark compat ack: %w", err)
		}
		return nil
	}

	start := time.Now()
	err := s.core.sendMessageToAgent(spanCtx, entry.AgentID, msg)
	attempt := entry.Attempt + 1
	if err != nil {
		s.telemetry.RecordError(spanCtx, err)
		s.recordDeliveryRetry(entry.AgentID, entry.Stage, pb.AckResult_ACK_RESULT_FAILED)
		return s.markPending(spanCtx, entry, err)
	}

	nextRetry := time.Now().Add(cfg.AckTimeout)
	entry.Attempt = attempt
	entry.Status = domain.DeliveryStatusInflight
	entry.LastError = ""
	entry.NextRetryAt = nextRetry

	if err := s.withJournalRetry(spanCtx, "delivery_journal.update_inflight", func(dbCtx context.Context) error {
		return s.repo.UpdateStatus(dbCtx, entry.CommandID, entry.Stage, entry.Status, entry.Attempt, nextRetry, "")
	}); err != nil {
		s.telemetry.RecordError(spanCtx, err)
		return fmt.Errorf("update inflight status: %w", err)
	}

	_ = s.recordRetryEvent(entry.CommandID, entry.Stage, pb.AckResult_ACK_RESULT_PENDING, attempt, time.Since(start))
	s.recordDeliveryRetry(entry.AgentID, entry.Stage, pb.AckResult_ACK_RESULT_OK)

	return nil
}

func (s *DeliveryService) ensureAgentAssignment(ctx context.Context, entry *domain.DeliveryJournalEntry) error {
	if entry.AgentID != pendingAgentPlaceholder && entry.AgentID != "" {
		return nil
	}
	if entry.TargetAccountID == "" {
		return fmt.Errorf("account not resolved")
	}
	owner, ok := s.core.accountRegistry.GetOwner(entry.TargetAccountID)
	if !ok || owner == "" {
		return fmt.Errorf("account owner unavailable")
	}
	if err := s.withJournalRetry(ctx, "delivery_journal.assign_agent", func(dbCtx context.Context) error {
		return s.repo.AssignAgent(dbCtx, entry.CommandID, owner)
	}); err != nil {
		return err
	}
	entry.AgentID = owner
	return nil
}

func (s *DeliveryService) markPending(ctx context.Context, entry *domain.DeliveryJournalEntry, cause error) error {
	entry.Attempt++
	cfg := s.getConfig()
	nextRetry := time.Now().Add(s.nextBackoff(cfg, entry.Attempt))
	entry.Status = domain.DeliveryStatusPending
	entry.LastError = ""
	if cause != nil {
		entry.LastError = cause.Error()
	}
	if entry.Attempt >= uint32(cfg.MaxRetries) {
		entry.Status = domain.DeliveryStatusFailed
		nextRetry = time.Time{}
	}
	if err := s.withJournalRetry(ctx, "delivery_journal.update_pending", func(dbCtx context.Context) error {
		return s.repo.UpdateStatus(dbCtx, entry.CommandID, entry.Stage, entry.Status, entry.Attempt, nextRetry, entry.LastError)
	}); err != nil {
		return fmt.Errorf("update pending status: %w", err)
	}
	_ = s.recordRetryEvent(entry.CommandID, entry.Stage, pb.AckResult_ACK_RESULT_FAILED, entry.Attempt, 0)
	s.recordDeliveryRetry(entry.AgentID, entry.Stage, pb.AckResult_ACK_RESULT_FAILED)
	return nil
}

func (s *DeliveryService) recordDeliveryRetry(agentID string, stage pb.AckStage, result pb.AckResult) {
	if s.core == nil || s.core.echoMetrics == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("agent_id", agentID),
	}
	s.core.echoMetrics.RecordDeliveryRetry(s.ctx, stage.String(), result.String(), attrs...)
}

func (s *DeliveryService) recordPendingAge(entry *domain.DeliveryJournalEntry, stage pb.AckStage) {
	if s.core == nil || s.core.echoMetrics == nil || entry == nil {
		return
	}
	age := time.Since(entry.CreatedAt)
	attrs := []attribute.KeyValue{
		attribute.String("agent_id", entry.AgentID),
	}
	s.core.echoMetrics.RecordDeliveryPendingAge(s.ctx, stage.String(), age, attrs...)
}

func (s *DeliveryService) nextBackoff(cfg DeliveryConfig, attempt uint32) time.Duration {
	if len(cfg.RetryBackoff) == 0 {
		return cfg.AckTimeout
	}
	index := int(attempt) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(cfg.RetryBackoff) {
		index = len(cfg.RetryBackoff) - 1
	}
	return cfg.RetryBackoff[index]
}

func (s *DeliveryService) recordRetryEvent(commandID string, stage pb.AckStage, result pb.AckResult, attempt uint32, duration time.Duration) error {
	if s.retryRepo == nil {
		return nil
	}
	event := &domain.DeliveryRetryEvent{
		CommandID:  commandID,
		Stage:      stage,
		Result:     result,
		Attempt:    attempt,
		DurationMs: duration.Milliseconds(),
	}
	return s.retryRepo.InsertEvent(s.ctx, event)
}

func commandIDFromMessage(msg *pb.CoreMessage) string {
	if msg == nil {
		return ""
	}
	switch payload := msg.Payload.(type) {
	case *pb.CoreMessage_ExecuteOrder:
		if payload.ExecuteOrder != nil {
			return payload.ExecuteOrder.CommandId
		}
	case *pb.CoreMessage_CloseOrder:
		if payload.CloseOrder != nil {
			return payload.CloseOrder.CommandId
		}
	}
	return ""
}

func (s *DeliveryService) withJournalRetry(ctx context.Context, operation string, fn func(context.Context) error) error {
	cfg := s.getConfig()
	maxAttempts := cfg.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 100
	}
	opCtx := s.ctx
	if opCtx == nil {
		opCtx = context.Background()
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = fn(opCtx)
		if lastErr == nil {
			if attempt > 1 {
				s.telemetry.Info(ctx, "delivery journal operation recovered",
					attribute.String("operation", operation),
					attribute.Int("attempts", attempt),
				)
			}
			return nil
		}

		if !s.isRetryableError(lastErr) {
			return fmt.Errorf("%s failed: %w", operation, lastErr)
		}

		backoff := s.nextBackoff(cfg, uint32(attempt))
		if backoff <= 0 {
			backoff = 100 * time.Millisecond
		}

		s.telemetry.Warn(ctx, "delivery journal operation retry",
			attribute.String("operation", operation),
			attribute.Int("attempt", attempt),
			attribute.String("error", lastErr.Error()),
			attribute.Int64("backoff_ms", backoff.Milliseconds()),
		)

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
			timer.Stop()
			continue
		case <-opCtx.Done():
			timer.Stop()
			return fmt.Errorf("%s aborted: %w", operation, opCtx.Err())
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, maxAttempts, lastErr)
}

func (s *DeliveryService) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	return true
}

type deliveryReconciler struct {
	service  *DeliveryService
	ticker   *time.Ticker
	done     chan struct{}
	updateCh chan time.Duration
}

func newDeliveryReconciler(service *DeliveryService) *deliveryReconciler {
	cfg := service.getConfig()
	interval := cfg.ReconcilerInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &deliveryReconciler{
		service:  service,
		ticker:   time.NewTicker(interval),
		done:     make(chan struct{}),
		updateCh: make(chan time.Duration, 1),
	}
}

func (r *deliveryReconciler) Start() error {
	go r.loop()
	return nil
}

func (r *deliveryReconciler) Stop() {
	r.ticker.Stop()
	close(r.done)
}

func (r *deliveryReconciler) loop() {
	for {
		select {
		case <-r.ticker.C:
			r.replayDue(domain.DeliveryStatusPending)
			r.replayDue(domain.DeliveryStatusInflight)
		case newInterval := <-r.updateCh:
			if newInterval <= 0 {
				newInterval = 500 * time.Millisecond
			}
			r.ticker.Stop()
			r.ticker = time.NewTicker(newInterval)
		case <-r.done:
			return
		case <-r.service.ctx.Done():
			return
		}
	}
}

func (r *deliveryReconciler) UpdateInterval(interval time.Duration) {
	if r == nil {
		return
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	select {
	case r.updateCh <- interval:
	default:
		select {
		case <-r.updateCh:
		default:
		}
		r.updateCh <- interval
	}
}

func (r *deliveryReconciler) replayDue(status domain.DeliveryStatus) {
	ctx := r.service.ctx
	entries, err := r.service.repo.GetDueEntries(ctx, status, time.Now(), deliveryBatchSize)
	if err != nil {
		r.service.telemetry.Warn(ctx, "delivery reconciler query failed",
			attribute.String("status", string(status)),
			attribute.String("error", err.Error()),
		)
		return
	}
	for _, entry := range entries {
		msg, err := decodeCoreMessage(entry.Payload)
		if err != nil {
			r.service.telemetry.Error(ctx, "failed to decode delivery payload", err,
				semconv.Echo.CommandID.String(entry.CommandID),
			)
			continue
		}
		if err := r.service.dispatch(ctx, entry, msg); err != nil {
			r.service.telemetry.Warn(ctx, "delivery dispatch retry failed",
				semconv.Echo.CommandID.String(entry.CommandID),
				attribute.String("error", err.Error()),
			)
		}
	}
}

func decodeCoreMessage(payload []byte) (*pb.CoreMessage, error) {
	var msg pb.CoreMessage
	if err := protojson.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
