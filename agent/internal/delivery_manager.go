package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/protojson"
)

type DeliveryManager struct {
	ctx        context.Context
	config     DeliveryConfig
	cfgMu      sync.RWMutex
	telemetry  *telemetry.Client
	metrics    *metricbundle.EchoMetrics
	ledger     *AckLedger
	sendCh     chan<- *pb.AgentMessage
	pipeLookup func(accountID string) (*PipeHandler, bool)
	agentID    string

	retryTicker *time.Ticker
}

func NewDeliveryManager(ctx context.Context, cfg *Config, tel *telemetry.Client, sendCh chan<- *pb.AgentMessage) (*DeliveryManager, error) {
	ledger, err := OpenAckLedger(cfg.Delivery.LedgerPath)
	if err != nil {
		return nil, err
	}
	metrics := tel.EchoMetrics()
	manager := &DeliveryManager{
		ctx:         ctx,
		config:      cfg.Delivery,
		telemetry:   tel,
		metrics:     metrics,
		ledger:      ledger,
		sendCh:      sendCh,
		agentID:     cfg.AgentID,
		retryTicker: time.NewTicker(500 * time.Millisecond),
	}
	go manager.retryLoop()
	return manager, nil
}

func (m *DeliveryManager) Close() {
	if m.retryTicker != nil {
		m.retryTicker.Stop()
	}
	if m.ledger != nil {
		_ = m.ledger.Close()
	}
}

func (m *DeliveryManager) SetPipeLookup(fn func(accountID string) (*PipeHandler, bool)) {
	m.pipeLookup = fn
}

func (m *DeliveryManager) HandleExecuteOrder(order *pb.ExecuteOrder) error {
	if order == nil {
		return fmt.Errorf("nil ExecuteOrder")
	}
	cfg := m.getConfig()
	payload, err := protojson.Marshal(order)
	if err != nil {
		m.telemetry.Error(m.ctx, "Failed to marshal ExecuteOrder for ledger", err)
		return fmt.Errorf("marshal execute order: %w", err)
	}
	record := &AckRecord{
		CommandID:       order.CommandId,
		TargetAccountID: order.TargetAccountId,
		Payload:         payload,
		PayloadType:     "execute",
		Stage:           pb.AckStage_ACK_STAGE_CORE_ACCEPTED,
		Attempt:         1,
		NextRetryAt:     time.Now().Add(cfg.AckTimeout).UnixMilli(),
		UpdatedAt:       time.Now().UnixMilli(),
	}
	if err := m.ledger.Put(record); err != nil {
		m.telemetry.Error(m.ctx, "Failed to persist delivery record", err)
		return fmt.Errorf("persist execute order: %w", err)
	}
	m.sendCommandAck(
		order.CommandId,
		pb.AckStage_ACK_STAGE_CORE_ACCEPTED,
		pb.AckResult_ACK_RESULT_PENDING,
		pb.ErrorCode_ERROR_CODE_UNSPECIFIED,
		record.Attempt,
	)
	return nil
}

func (m *DeliveryManager) HandleCloseOrder(order *pb.CloseOrder) error {
	if order == nil {
		return fmt.Errorf("nil CloseOrder")
	}
	cfg := m.getConfig()
	payload, err := protojson.Marshal(order)
	if err != nil {
		return fmt.Errorf("marshal close order: %w", err)
	}
	record := &AckRecord{
		CommandID:       order.CommandId,
		TargetAccountID: order.TargetAccountId,
		Payload:         payload,
		PayloadType:     "close",
		Stage:           pb.AckStage_ACK_STAGE_CORE_ACCEPTED,
		Attempt:         1,
		NextRetryAt:     time.Now().Add(cfg.AckTimeout).UnixMilli(),
		UpdatedAt:       time.Now().UnixMilli(),
	}
	if err := m.ledger.Put(record); err != nil {
		return fmt.Errorf("persist close order: %w", err)
	}
	m.sendCommandAck(
		order.CommandId,
		pb.AckStage_ACK_STAGE_CORE_ACCEPTED,
		pb.AckResult_ACK_RESULT_PENDING,
		pb.ErrorCode_ERROR_CODE_UNSPECIFIED,
		record.Attempt,
	)
	return nil
}

func (m *DeliveryManager) HandlePipeResult(commandID string, success bool, cause error) {
	entry, err := m.ledger.Get(commandID)
	if err != nil || entry == nil {
		return
	}
	cfg := m.getConfig()

	attempt := entry.Attempt
	if attempt == 0 {
		attempt = 1
	}

	nextRetry := time.Now().Add(cfg.AckTimeout)
	lastError := ""
	if !success && cause != nil {
		lastError = cause.Error()
	}

	entry.Stage = pb.AckStage_ACK_STAGE_AGENT_BUFFERED
	result := pb.AckResult_ACK_RESULT_PENDING

	if !success {
		if attempt < uint32(cfg.MaxRetries) {
			attempt++
			nextRetry = time.Now().Add(m.backoff(cfg, attempt))
		} else {
			// Exceeded retries, mark as failed and stop scheduling retries.
			result = pb.AckResult_ACK_RESULT_FAILED
			nextRetry = time.Time{}
		}
		m.recordPipeRetry(entry.TargetAccountID, entry.Stage, result.String())
	}

	_ = m.ledger.UpdateStage(commandID, entry.Stage, attempt, nextRetry, lastError)

	if success {
		m.sendCommandAck(
			commandID,
			pb.AckStage_ACK_STAGE_AGENT_BUFFERED,
			pb.AckResult_ACK_RESULT_PENDING,
			pb.ErrorCode_ERROR_CODE_UNSPECIFIED,
			attempt,
		)
		return
	}

	if result == pb.AckResult_ACK_RESULT_FAILED {
		m.sendCommandAck(
			commandID,
			pb.AckStage_ACK_STAGE_AGENT_BUFFERED,
			pb.AckResult_ACK_RESULT_FAILED,
			pb.ErrorCode_ERROR_CODE_UNSPECIFIED,
			attempt,
		)
	}
}

func (m *DeliveryManager) recordPipeRetry(accountID string, stage pb.AckStage, result string) {
	if m.metrics == nil {
		return
	}
	stageName := stage.String()
	if stage == pb.AckStage_ACK_STAGE_UNSPECIFIED {
		stageName = "unspecified"
	}
	m.metrics.RecordAgentPipeRetry(
		m.ctx,
		m.agentID,
		accountID,
		stageName,
		result,
	)
}

func (m *DeliveryManager) HandlePipeDeliveryAck(ack *pb.PipeDeliveryAck) {
	if ack == nil || ack.CommandId == "" {
		return
	}
	cfg := m.getConfig()
	entry, err := m.ledger.Get(ack.CommandId)
	if err != nil || entry == nil {
		return
	}
	attempt := entry.Attempt
	if attempt == 0 {
		attempt = 1
	}

	switch ack.Result {
	case pb.PipeDeliveryAckResult_PIPE_DELIVERY_ACK_RESULT_OK:
		_ = m.ledger.UpdateStage(
			ack.CommandId,
			pb.AckStage_ACK_STAGE_PIPE_DELIVERED,
			attempt,
			time.Now().Add(cfg.AckTimeout),
			ack.ErrorMessage,
		)
		m.sendCommandAck(
			ack.CommandId,
			pb.AckStage_ACK_STAGE_PIPE_DELIVERED,
			pb.AckResult_ACK_RESULT_OK,
			pb.ErrorCode_ERROR_CODE_UNSPECIFIED,
			attempt,
		)
	case pb.PipeDeliveryAckResult_PIPE_DELIVERY_ACK_RESULT_RETRY:
		if attempt < uint32(cfg.MaxRetries) {
			attempt++
		}
		_ = m.ledger.UpdateStage(
			ack.CommandId,
			pb.AckStage_ACK_STAGE_AGENT_BUFFERED,
			attempt,
			time.Now().Add(m.backoff(cfg, attempt)),
			ack.ErrorMessage,
		)
		m.recordPipeRetry(entry.TargetAccountID, pb.AckStage_ACK_STAGE_AGENT_BUFFERED, ack.Result.String())
	case pb.PipeDeliveryAckResult_PIPE_DELIVERY_ACK_RESULT_FAILED:
		_ = m.ledger.UpdateStage(
			ack.CommandId,
			pb.AckStage_ACK_STAGE_AGENT_BUFFERED,
			attempt,
			time.Time{},
			ack.ErrorMessage,
		)
		m.sendCommandAck(
			ack.CommandId,
			pb.AckStage_ACK_STAGE_AGENT_BUFFERED,
			pb.AckResult_ACK_RESULT_FAILED,
			pb.ErrorCode_ERROR_CODE_UNSPECIFIED,
			attempt,
		)
		m.recordPipeRetry(entry.TargetAccountID, pb.AckStage_ACK_STAGE_AGENT_BUFFERED, ack.Result.String())
	default:
		// ignore
	}
}

func (m *DeliveryManager) HandleExecutionResult(result *pb.ExecutionResult) {
	if result == nil {
		return
	}
	m.sendCommandAck(result.CommandId, pb.AckStage_ACK_STAGE_EA_CONFIRMED, ackResultFromSuccess(result.Success), result.ErrorCode, 0)
	_ = m.ledger.Delete(result.CommandId)
}

func (m *DeliveryManager) UpdateConfig(hb *pb.DeliveryHeartbeat) {
	if hb == nil {
		return
	}
	cfg := m.getConfig()
	if hb.AckTimeoutMs > 0 {
		cfg.AckTimeout = time.Duration(hb.AckTimeoutMs) * time.Millisecond
	} else if hb.HeartbeatIntervalMs > 0 && cfg.AckTimeout <= 0 {
		// Fallback para compatibilidad con versiones anteriores.
		cfg.AckTimeout = time.Duration(hb.HeartbeatIntervalMs) * time.Millisecond
	}
	if hb.MaxRetries > 0 {
		cfg.MaxRetries = int(hb.MaxRetries)
	}
	if len(hb.RetryBackoffMs) > 0 {
		backoff := make([]time.Duration, 0, len(hb.RetryBackoffMs))
		for _, ms := range hb.RetryBackoffMs {
			if ms <= 0 {
				continue
			}
			backoff = append(backoff, time.Duration(ms)*time.Millisecond)
		}
		if len(backoff) > 0 {
			cfg.RetryBackoff = backoff
		}
	}
	m.setConfig(cfg)
}

func (m *DeliveryManager) retryLoop() {
	limit := 32
	for {
		select {
		case <-m.retryTicker.C:
			due, err := m.ledger.ListDue(time.Now(), limit)
			if err != nil {
				continue
			}
			for _, rec := range due {
				m.retryDelivery(rec)
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *DeliveryManager) retryDelivery(rec *AckRecord) {
	if rec == nil {
		return
	}
	cfg := m.getConfig()
	attempt := rec.Attempt
	if attempt == 0 {
		attempt = 1
	}
	spanCtx, span := m.telemetry.StartSpan(m.ctx, "agent.pipe.delivery")
	span.SetAttributes(
		semconv.Echo.CommandID.String(rec.CommandID),
		attribute.String("target_account_id", rec.TargetAccountID),
		attribute.String("payload_type", rec.PayloadType),
		attribute.Int("attempt", int(attempt)),
		attribute.Int64("ack_timeout_ms", cfg.AckTimeout.Milliseconds()),
		attribute.Int64("backoff_ms", m.backoff(cfg, attempt+1).Milliseconds()),
		attribute.String("agent_id", m.agentID),
	)
	defer span.End()
	handler, ok := m.lookupHandler(rec.TargetAccountID)
	if !ok || handler == nil {
		span.AddEvent("pipe_handler_missing", trace.WithAttributes(attribute.String("account_id", rec.TargetAccountID)))
		return
	}

	switch rec.PayloadType {
	case "execute":
		var order pb.ExecuteOrder
		if err := protojson.Unmarshal(rec.Payload, &order); err != nil {
			m.telemetry.RecordError(spanCtx, err)
			return
		}
		start := time.Now()
		if err := handler.WriteMessage(&order); err != nil {
			m.telemetry.RecordError(spanCtx, err)
			m.recordPipeDeliveryLatency(rec.TargetAccountID, "error", time.Since(start))
			m.HandlePipeResult(order.CommandId, false, err)
			return
		}
		m.recordPipeDeliveryLatency(rec.TargetAccountID, "ok", time.Since(start))
		span.SetAttributes(attribute.String("result", "ok"))
		m.HandlePipeResult(order.CommandId, true, nil)
	case "close":
		var order pb.CloseOrder
		if err := protojson.Unmarshal(rec.Payload, &order); err != nil {
			m.telemetry.RecordError(spanCtx, err)
			return
		}
		start := time.Now()
		if err := handler.WriteMessage(&order); err != nil {
			m.telemetry.RecordError(spanCtx, err)
			m.recordPipeDeliveryLatency(rec.TargetAccountID, "error", time.Since(start))
			m.HandlePipeResult(order.CommandId, false, err)
			return
		}
		m.recordPipeDeliveryLatency(rec.TargetAccountID, "ok", time.Since(start))
		span.SetAttributes(attribute.String("result", "ok"))
		m.HandlePipeResult(order.CommandId, true, nil)
	}
}

func (m *DeliveryManager) lookupHandler(accountID string) (*PipeHandler, bool) {
	if m.pipeLookup == nil {
		return nil, false
	}
	return m.pipeLookup(accountID)
}

func (m *DeliveryManager) getConfig() DeliveryConfig {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.config
}

func (m *DeliveryManager) setConfig(cfg DeliveryConfig) {
	m.cfgMu.Lock()
	m.config = cfg
	m.cfgMu.Unlock()
}

func (m *DeliveryManager) sendCommandAck(commandID string, stage pb.AckStage, result pb.AckResult, errorCode pb.ErrorCode, attempt uint32) {
	if m.sendCh == nil || commandID == "" {
		return
	}
	cfg := m.getConfig()
	ack := &pb.CommandAck{
		CommandId:    commandID,
		Stage:        stage,
		Result:       result,
		Attempt:      attempt,
		ObservedAtMs: utils.NowUnixMilli(),
		ErrorCode:    errorCode,
	}
	msg := &pb.AgentMessage{
		TimestampMs: utils.NowUnixMilli(),
		Payload: &pb.AgentMessage_CommandAck{
			CommandAck: ack,
		},
	}
	timeout := cfg.AckTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case m.sendCh <- msg:
			return
		case <-m.ctx.Done():
			return
		case <-timer.C:
			m.telemetry.Warn(m.ctx, "send channel congested while emitting CommandAck",
				semconv.Echo.CommandID.String(commandID),
				attribute.String("stage", stage.String()),
				attribute.String("result", result.String()),
				attribute.Int("attempt", int(attempt)),
			)
			timer.Reset(timeout)
		}
	}
}

func (m *DeliveryManager) backoff(cfg DeliveryConfig, attempt uint32) time.Duration {
	if len(cfg.RetryBackoff) == 0 {
		return cfg.AckTimeout
	}
	idx := int(attempt) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cfg.RetryBackoff) {
		idx = len(cfg.RetryBackoff) - 1
	}
	return cfg.RetryBackoff[idx]
}

func ackResultFromSuccess(success bool) pb.AckResult {
	if success {
		return pb.AckResult_ACK_RESULT_OK
	}
	return pb.AckResult_ACK_RESULT_FAILED
}

func (m *DeliveryManager) recordPipeDeliveryLatency(accountID string, result string, duration time.Duration) {
	if m.metrics == nil {
		return
	}
	m.metrics.RecordAgentPipeDeliveryLatency(m.ctx, m.agentID, accountID, result, duration)
}
