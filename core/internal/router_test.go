package internal

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestRouter_extractTradeID(t *testing.T) {
	router := &Router{}

	cases := []struct {
		name string
		msg  *pb.AgentMessage
		want string
	}{
		{
			name: "trade_intent",
			msg: &pb.AgentMessage{
				Payload: &pb.AgentMessage_TradeIntent{
					TradeIntent: &pb.TradeIntent{TradeId: "ABC-123"},
				},
			},
			want: "abc-123",
		},
		{
			name: "execution_result",
			msg: &pb.AgentMessage{
				Payload: &pb.AgentMessage_ExecutionResult{
					ExecutionResult: &pb.ExecutionResult{TradeId: "XYZ"},
				},
			},
			want: "xyz",
		},
		{
			name: "trade_close",
			msg: &pb.AgentMessage{
				Payload: &pb.AgentMessage_TradeClose{
					TradeClose: &pb.TradeClose{TradeId: "CloseID"},
				},
			},
			want: "closeid",
		},
		{
			name: "nil_payload",
			msg:  &pb.AgentMessage{},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := router.extractTradeID(tc.msg)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestHashTradeIDDeterministic(t *testing.T) {
	value := "abc-123"
	first := hashTradeID(value)
	second := hashTradeID(value)

	if first != second {
		t.Fatalf("hashTradeID should be deterministic, got %d and %d", first, second)
	}
}

func TestSendBackpressureAckRetriesUntilChannelAvailable(t *testing.T) {
	tel := newTestTelemetryClient(t)
	router := &Router{
		core: &Core{
			telemetry: tel,
			ctx:       context.Background(),
			agents:    make(map[string]*AgentConnection),
		},
		workerTimeout: 20 * time.Millisecond,
		ctx:           context.Background(),
	}
	agent := &AgentConnection{
		AgentID: "agent-1",
		SendCh:  make(chan *pb.CoreMessage),
	}
	router.core.agents[agent.AgentID] = agent

	received := make(chan *pb.CoreMessage, 1)
	go func() {
		time.Sleep(2 * router.workerTimeout)
		msg := <-agent.SendCh
		received <- msg
	}()

	start := time.Now()
	router.sendBackpressureAck(context.Background(), agent.AgentID, "trade-1", 3, 0)
	elapsed := time.Since(start)
	if elapsed < router.workerTimeout {
		t.Fatalf("expected sendBackpressureAck to wait at least %s, got %s", router.workerTimeout, elapsed)
	}

	select {
	case ack := <-received:
		if ack.GetAck() == nil {
			t.Fatalf("expected ack payload, got nil")
		}
		if ack.GetAck().GetSuccess() {
			t.Fatalf("expected ack.Success=false")
		}
		if errText := ack.GetAck().GetError(); errText != pb.ErrorCode_ERROR_CODE_BROKER_BUSY.String() {
			t.Fatalf("expected error %s, got %s", pb.ErrorCode_ERROR_CODE_BROKER_BUSY.String(), errText)
		}
	case <-time.After(time.Second):
		t.Fatal("backpressure ack not delivered")
	}
}

func TestRejectTradeIntentBackpressurePersistsAndSendsAck(t *testing.T) {
	tel := newTestTelemetryClient(t)
	dedupeRepo := newTestDedupeRepo()
	tradeRepo := newTestTradeRepo()
	core := &Core{
		telemetry:     tel,
		echoMetrics:   newTestEchoMetrics(t),
		dedupeService: NewDedupeService(dedupeRepo),
		repoFactory: &testRepoFactory{
			tradeRepo: tradeRepo,
		},
		agents: make(map[string]*AgentConnection),
		ctx:    context.Background(),
	}
	router := &Router{
		core:          core,
		workerTimeout: 10 * time.Millisecond,
		ctx:           context.Background(),
		workers: []*routerWorker{
			{queue: make(chan *routerMessage, 1)},
		},
	}

	agent := &AgentConnection{
		AgentID: "agent-1",
		SendCh:  make(chan *pb.CoreMessage, 1),
	}
	router.core.agents[agent.AgentID] = agent

	intent := &pb.TradeIntent{
		TradeId:     "TRADE-ABC",
		ClientId:    "master-1",
		Symbol:      "XAUUSD",
		LotSize:     0.1,
		MagicNumber: 42,
		TimestampMs: 12345,
		StrategyId:  "default",
	}

	msg := &routerMessage{
		ctx:      context.Background(),
		agentID:  agent.AgentID,
		workerID: 0,
	}

	router.rejectTradeIntentBackpressure(msg, intent, 0)

	tradeKey := strings.ToLower(intent.TradeId)
	if status, ok := dedupeRepo.Status(tradeKey); !ok || status != domain.OrderStatusRejected {
		t.Fatalf("expected dedupe entry with status REJECTED, got %v (exists=%v)", status, ok)
	}

	if len(tradeRepo.Created()) != 1 {
		t.Fatalf("expected 1 persisted trade, got %d", len(tradeRepo.Created()))
	}

	select {
	case ack := <-agent.SendCh:
		if ack.GetAck() == nil || ack.GetAck().GetError() != pb.ErrorCode_ERROR_CODE_BROKER_BUSY.String() {
			t.Fatalf("unexpected ack payload: %+v", ack.GetAck())
		}
	default:
		t.Fatal("expected backpressure ack in agent channel")
	}
}

// --- Test helpers ---

func newTestTelemetryClient(t *testing.T) *telemetry.Client {
	t.Helper()
	ctx := context.Background()
	client, err := telemetry.New(ctx, "router-test", "test",
		telemetry.WithLogsDisabled(),
		telemetry.WithMetricsDisabled(),
		telemetry.WithTracesDisabled(),
	)
	if err != nil {
		t.Fatalf("failed to init telemetry: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Shutdown(shutdownCtx)
	})
	return client
}

type testDedupeRepo struct {
	mu      sync.Mutex
	entries map[string]domain.OrderStatus
}

func newTestDedupeRepo() *testDedupeRepo {
	return &testDedupeRepo{
		entries: make(map[string]domain.OrderStatus),
	}
}

func (r *testDedupeRepo) Upsert(_ context.Context, entry *domain.DedupeEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[entry.TradeID] = entry.Status
	return nil
}

func (r *testDedupeRepo) Get(_ context.Context, tradeID string) (*domain.DedupeEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.entries[tradeID]
	if !ok {
		return nil, nil
	}
	return &domain.DedupeEntry{TradeID: tradeID, Status: status}, nil
}

func (r *testDedupeRepo) Exists(_ context.Context, tradeID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[tradeID]
	return ok, nil
}

func (r *testDedupeRepo) UpdateStatus(_ context.Context, tradeID string, status domain.OrderStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[tradeID] = status
	return nil
}

func (r *testDedupeRepo) CleanupTTL(context.Context) (int, error) {
	return 0, nil
}

func (r *testDedupeRepo) Status(tradeID string) (domain.OrderStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.entries[tradeID]
	return status, ok
}

type testTradeRepo struct {
	mu      sync.Mutex
	created []*domain.Trade
}

func newTestTradeRepo() *testTradeRepo {
	return &testTradeRepo{
		created: make([]*domain.Trade, 0),
	}
}

func (r *testTradeRepo) Create(_ context.Context, trade *domain.Trade) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := *trade
	r.created = append(r.created, &cloned)
	return nil
}

func (r *testTradeRepo) Created() []*domain.Trade {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.Trade, len(r.created))
	copy(out, r.created)
	return out
}

func (r *testTradeRepo) GetByID(context.Context, string) (*domain.Trade, error) { return nil, nil }
func (r *testTradeRepo) GetByMasterTicket(context.Context, string, int32) (*domain.Trade, error) {
	return nil, nil
}
func (r *testTradeRepo) UpdateStatus(context.Context, string, domain.OrderStatus) error { return nil }
func (r *testTradeRepo) List(context.Context, int, int) ([]*domain.Trade, error)        { return nil, nil }
func (r *testTradeRepo) ListByStatus(context.Context, domain.OrderStatus, int, int) ([]*domain.Trade, error) {
	return nil, nil
}

type testRepoFactory struct {
	tradeRepo domain.TradeRepository
}

func (f *testRepoFactory) TradeRepository() domain.TradeRepository { return f.tradeRepo }
func (f *testRepoFactory) ExecutionRepository() domain.ExecutionRepository {
	return nil
}
func (f *testRepoFactory) DedupeRepository() domain.DedupeRepository { return nil }
func (f *testRepoFactory) CloseRepository() domain.CloseRepository   { return nil }
func (f *testRepoFactory) CorrelationService() domain.CorrelationService {
	return nil
}
func (f *testRepoFactory) SymbolRepository() domain.SymbolRepository { return nil }
func (f *testRepoFactory) SymbolSpecRepository() domain.SymbolSpecRepository {
	return nil
}
func (f *testRepoFactory) SymbolQuoteRepository() domain.SymbolQuoteRepository {
	return nil
}
func (f *testRepoFactory) RiskPolicyRepository() domain.RiskPolicyRepository { return nil }
func (f *testRepoFactory) HandshakeRepository() domain.HandshakeEvaluationRepository {
	return nil
}
func (f *testRepoFactory) DeliveryJournalRepository() domain.DeliveryJournalRepository { return nil }
func (f *testRepoFactory) DeliveryRetryEventRepository() domain.DeliveryRetryEventRepository {
	return nil
}

func newTestEchoMetrics(t *testing.T) *metricbundle.EchoMetrics {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	meter := provider.Meter("router-test")
	metrics, err := metricbundle.NewEchoMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create echo metrics: %v", err)
	}
	return metrics
}
