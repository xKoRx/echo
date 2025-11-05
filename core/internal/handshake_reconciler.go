package internal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

type handshakeReconciler struct {
	evaluator       *HandshakeEvaluator
	resolver        *AccountSymbolResolver
	registry        *HandshakeRegistry
	accountRegistry *AccountRegistry
	repository      domain.HandshakeEvaluationRepository
	telemetry       *telemetry.Client
	metrics         *metricbundle.EchoMetrics
	coreVersion     string
	sendFunc        func(agentID string, result *pb.SymbolRegistrationResult) error

	queue   chan string
	pending map[string]struct{}
	mu      sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	listenerMu sync.Mutex
	listener   *pq.Listener
}

func newHandshakeReconciler(
	parentCtx context.Context,
	evaluator *HandshakeEvaluator,
	resolver *AccountSymbolResolver,
	registry *HandshakeRegistry,
	accountRegistry *AccountRegistry,
	repository domain.HandshakeEvaluationRepository,
	telemetryClient *telemetry.Client,
	metrics *metricbundle.EchoMetrics,
	coreVersion string,
	sendFunc func(agentID string, result *pb.SymbolRegistrationResult) error,
) *handshakeReconciler {
	ctx, cancel := context.WithCancel(parentCtx)
	return &handshakeReconciler{
		evaluator:       evaluator,
		resolver:        resolver,
		registry:        registry,
		accountRegistry: accountRegistry,
		repository:      repository,
		telemetry:       telemetryClient,
		metrics:         metrics,
		coreVersion:     coreVersion,
		sendFunc:        sendFunc,
		queue:           make(chan string, 1024),
		pending:         make(map[string]struct{}),
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (r *handshakeReconciler) Start() {
	r.wg.Add(1)
	go r.worker()
}

func (r *handshakeReconciler) Stop() {
	r.cancel()
	r.closeListener()
	r.wg.Wait()
}

func (r *handshakeReconciler) Notify(accountID string) {
	if accountID == "" {
		return
	}
	r.mu.Lock()
	if _, exists := r.pending[accountID]; exists {
		r.mu.Unlock()
		return
	}
	r.pending[accountID] = struct{}{}
	r.mu.Unlock()

	select {
	case r.queue <- accountID:
	default:
		r.mu.Lock()
		delete(r.pending, accountID)
		r.mu.Unlock()
		r.telemetry.Warn(r.ctx, "Handshake reconciler queue full",
			semconv.Echo.AccountID.String(accountID),
		)
	}
}

func (r *handshakeReconciler) worker() {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case accountID := <-r.queue:
			r.mu.Lock()
			delete(r.pending, accountID)
			r.mu.Unlock()

			if _, err := r.process(r.ctx, accountID, true); err != nil {
				r.telemetry.Warn(r.ctx, "Handshake re-evaluation failed",
					attribute.String("account_id", accountID),
					attribute.String("error", err.Error()),
				)
			}
		}
	}
}

func (r *handshakeReconciler) process(ctx context.Context, accountID string, send bool) (*handshake.Evaluation, error) {
	if r.evaluator == nil || r.repository == nil {
		return nil, fmt.Errorf("evaluator no inicializado")
	}

	if ctx == nil {
		ctx = r.ctx
	}

	ctx, span := r.telemetry.StartSpan(ctx, "core.handshake.reconcile")
	defer span.End()

	evalSnapshot, err := r.repository.GetLatestByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("obtener evaluación previa: %w", err)
	}
	if evalSnapshot == nil {
		return nil, nil
	}

	metadata := &pb.HandshakeMetadata{
		ProtocolVersion:  int32(evalSnapshot.ProtocolVersion),
		ClientSemver:     evalSnapshot.ClientSemver,
		RequiredFeatures: append([]string(nil), evalSnapshot.RequiredFeatures...),
		OptionalFeatures: append([]string(nil), evalSnapshot.OptionalFeatures...),
	}
	if len(evalSnapshot.Capabilities.Features) > 0 || len(evalSnapshot.Capabilities.Metrics) > 0 {
		metadata.Capabilities = &pb.HandshakeCapabilities{
			Features: append([]string(nil), evalSnapshot.Capabilities.Features...),
			Metrics:  append([]string(nil), evalSnapshot.Capabilities.Metrics...),
		}
	}

	mappings, err := r.resolver.ListMappings(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("obtener mappings: %w", err)
	}

	agentID, _ := r.accountRegistry.GetOwner(accountID)
	pipeRole := evalSnapshot.PipeRole

	newEvaluation, persisted, err := r.evaluator.EvaluateWithPrevious(ctx, accountID, agentID, pipeRole, r.coreVersion, metadata, mappings, evalSnapshot)
	if err != nil {
		return nil, fmt.Errorf("evaluar handshake: %w", err)
	}

	current := newEvaluation
	if current == nil {
		current = evalSnapshot
	}

	if current == nil {
		return nil, nil
	}

	r.registry.Set(current)

	if !persisted {
		if r.metrics != nil {
			r.metrics.RecordHandshakeReconcileSkipped(ctx,
				semconv.Echo.AccountID.String(accountID),
				attribute.String("pipe_role", pipeRole),
			)
		}

		r.telemetry.Debug(ctx, "Handshake re-evaluation reused previous result",
			semconv.Echo.AccountID.String(accountID),
			attribute.String("pipe_role", pipeRole),
			attribute.String("status", registrationStatusString(current.Status)),
		)
	} else {
		r.telemetry.Info(ctx, "Handshake re-evaluado",
			semconv.Echo.AccountID.String(accountID),
			attribute.String("pipe_role", pipeRole),
			attribute.String("status", registrationStatusString(current.Status)),
		)
	}

	if send && agentID != "" && r.sendFunc != nil {
		if err := r.sendFunc(agentID, current.ToProtoResult()); err != nil {
			r.metrics.RecordAgentHandshakeForwardError(ctx, err.Error(),
				semconv.Echo.AccountID.String(accountID),
				attribute.String("pipe_role", pipeRole),
			)
			return current, fmt.Errorf("enviar resultado a agent %s: %w", agentID, err)
		}
	}

	return current, nil
}

func (r *handshakeReconciler) EvaluateNow(ctx context.Context, accountID string, send bool) (*handshake.Evaluation, error) {
	return r.process(ctx, accountID, send)
}

func (r *handshakeReconciler) StartListener(connStr string) error {
	if connStr == "" {
		return nil
	}

	r.listenerMu.Lock()
	defer r.listenerMu.Unlock()

	if r.listener != nil {
		return nil
	}

	listener := pq.NewListener(connStr, 5*time.Second, time.Minute, nil)
	if err := listener.Listen("echo_handshake_result"); err != nil {
		listener.Close()
		return err
	}

	r.listener = listener

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-r.ctx.Done():
				return
			case notification := <-listener.Notify:
				if notification == nil {
					continue
				}
				accountID := parseHandshakeNotification(notification.Extra)
				if accountID != "" {
					r.Notify(accountID)
				}
			}
		}
	}()

	return nil
}

func (r *handshakeReconciler) closeListener() {
	r.listenerMu.Lock()
	if r.listener != nil {
		r.listener.Close()
		r.listener = nil
	}
	r.listenerMu.Unlock()
}

func parseHandshakeNotification(payload string) string {
	parts := strings.Split(payload, ":")
	if len(parts) < 1 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
