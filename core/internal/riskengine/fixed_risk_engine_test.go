package riskengine

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xKoRx/echo/core/internal/volumeguard"
	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"google.golang.org/protobuf/proto"
)

type stubSpecProvider struct {
	Specs map[string]*pb.SymbolSpecification
}

func (s *stubSpecProvider) GetSpecification(ctx context.Context, accountID, canonical string) (*pb.SymbolSpecification, int64, bool) {
	if s.Specs == nil {
		return nil, 0, false
	}
	key := accountID + "::" + canonical
	spec, ok := s.Specs[key]
	if !ok {
		return nil, 0, false
	}
	return proto.Clone(spec).(*pb.SymbolSpecification), time.Now().UnixMilli(), true
}

type stubSymbolProvider struct {
	Infos map[string]*domain.AccountSymbolInfo
}

func (s *stubSymbolProvider) Resolve(ctx context.Context, accountID, canonical string) (*domain.AccountSymbolInfo, bool) {
	if s.Infos == nil {
		return nil, false
	}
	key := accountID + "::" + canonical
	info, ok := s.Infos[key]
	if !ok {
		return nil, false
	}
	clone := *info
	return &clone, true
}

type stubQuoteProvider struct {
	Quotes map[string]*pb.SymbolQuoteSnapshot
}

func (s *stubQuoteProvider) Get(accountID, canonical string) (*pb.SymbolQuoteSnapshot, bool) {
	if s.Quotes == nil {
		return nil, false
	}
	key := accountID + "::" + canonical
	q, ok := s.Quotes[key]
	if !ok {
		return nil, false
	}
	return proto.Clone(q).(*pb.SymbolQuoteSnapshot), true
}

type stubAccountState struct {
	Accounts map[string]*pb.AccountInfo
}

func (s *stubAccountState) Get(accountID string) (*pb.AccountInfo, bool) {
	if s.Accounts == nil {
		return nil, false
	}
	info, ok := s.Accounts[accountID]
	if !ok {
		return nil, false
	}
	return proto.Clone(info).(*pb.AccountInfo), true
}

type stubGuard struct {
	lot      float64
	decision volumeguard.Decision
	returned error
}

func (s *stubGuard) Execute(ctx context.Context, accountID, canonicalSymbol, strategyID string, requestedLot float64) (float64, volumeguard.Decision, error) {
	if s.decision == "" {
		return requestedLot, volumeguard.DecisionPassThrough, nil
	}
	lot := requestedLot
	if s.lot > 0 {
		lot = s.lot
	}
	return lot, s.decision, s.returned
}

func buildSpec(digits int32, tickValue float64) *pb.SymbolSpecification {
	return &pb.SymbolSpecification{
		General: &pb.SymbolGeneral{
			Digits:    digits,
			TickValue: tickValue,
		},
	}
}

func buildQuote(price float64, timestamp time.Time) *pb.SymbolQuoteSnapshot {
	return &pb.SymbolQuoteSnapshot{
		Bid:         price,
		Ask:         price,
		TimestampMs: timestamp.UnixMilli(),
	}
}

func TestFixedRiskEngine_ComputeLot_Success(t *testing.T) {
	spec := buildSpec(2, 1.0)
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": spec,
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(4000, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01},
		}},
		&stubGuard{},
		Config{MaxQuoteAge: time.Second, MinDistancePoints: 5, MaxRiskDrift: 0.02, DefaultCurrency: "USD", RejectOnMissingTickValue: true},
		nil,
		nil,
	)

	price := 4000.0
	stop := 3950.0
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop}
	policy := &domain.FixedRiskConfig{Amount: 100, Currency: "USD"}

	result, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	require.NoError(t, err)
	assert.Equal(t, DecisionProceed, result.Decision)
	assert.InDelta(t, 0.02, result.Lot, 1e-6)
	assert.InDelta(t, 100, result.ExpectedLoss, 1e-2)
}

func TestFixedRiskEngine_CommissionCosts(t *testing.T) {
	spec := buildSpec(2, 1.0)
	spec.General.ContractSize = 100
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": spec,
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(3977.55, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01, ContractSize: floatPtr(100)},
		}},
		&stubGuard{},
		Config{MaxQuoteAge: time.Second, MinDistancePoints: 5, MaxRiskDrift: 0.02, DefaultCurrency: "USD", RejectOnMissingTickValue: true},
		nil,
		nil,
	)

	price := 3977.55
	stop := 3927.55 // distancia de 50.0 puntos
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop, Side: pb.OrderSide_ORDER_SIDE_BUY}
	commissionFixed := 15.65
	commissionRatePercent := 0.5
	commissionRateFactor := commissionRatePercent / 100.0
	policy := &domain.FixedRiskConfig{
		Amount:           150,
		Currency:         "USD",
		CommissionPerLot: floatPtr(commissionFixed),
		CommissionRate:   floatPtr(commissionRatePercent),
	}

	result, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	require.NoError(t, err)
	assert.Equal(t, DecisionProceed, result.Decision)

	orderValueCost := commissionRateFactor * price * 100
	totalCostPerLot := commissionFixed + orderValueCost
	assert.InDelta(t, commissionFixed, result.CommissionFixedPerLot, 1e-9)
	assert.InDelta(t, totalCostPerLot, result.CommissionPerLot, 1e-9)
	assert.InDelta(t, commissionRateFactor, result.CommissionRate, 1e-9)
	assert.InDelta(t, 150.0, result.ExpectedLoss, 1e-2)
	assert.InDelta(t, totalCostPerLot*result.Lot, result.CommissionTotal, 1e-6)
}

func TestFixedRiskEngine_MinLotOverride(t *testing.T) {
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": buildSpec(2, 1.0),
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(4000, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01},
		}},
		&stubGuard{},
		Config{MaxQuoteAge: time.Second, MinDistancePoints: 5, MaxRiskDrift: 0.02, DefaultCurrency: "USD", RejectOnMissingTickValue: true},
		nil,
		nil,
	)

	price := 4000.0
	stop := 3999.0
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop, Side: pb.OrderSide_ORDER_SIDE_BUY}
	minOverride := 0.2
	policy := &domain.FixedRiskConfig{Amount: 5, Currency: "USD", MinLotOverride: floatPtr(minOverride)}

	result, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	require.NoError(t, err)
	assert.Equal(t, DecisionProceed, result.Decision)
	assert.InDelta(t, minOverride, result.Lot, 1e-9)

	distancePoints := math.Abs(price-stop) / 0.01
	expected := minOverride * distancePoints * 1.0
	assert.InDelta(t, expected, result.ExpectedLoss, 1e-6)
}

func TestFixedRiskEngine_GuardClampTolerance(t *testing.T) {
	spec := buildSpec(2, 1.0)
	spec.General.ContractSize = 100
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": spec,
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(3977.55, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01, ContractSize: floatPtr(100)},
		}},
		&stubGuard{
			lot:      0.03,
			decision: volumeguard.DecisionClamp,
		},
		Config{MaxQuoteAge: time.Second, MinDistancePoints: 5, MaxRiskDrift: 0.02, DefaultCurrency: "USD", RejectOnMissingTickValue: true},
		nil,
		nil,
	)

	price := 3977.55
	stop := price - 28.446
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop, Side: pb.OrderSide_ORDER_SIDE_BUY}
	policy := &domain.FixedRiskConfig{
		Amount:         150,
		Currency:       "USD",
		CommissionRate: floatPtr(0.5),
	}

	result, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	require.NoError(t, err)
	assert.Equal(t, DecisionProceed, result.Decision)
	assert.InDelta(t, 0.03, result.Lot, 1e-9)
	assert.True(t, result.ExpectedLoss < policy.Amount)
	assert.InDelta(t, 0.5/100.0, result.CommissionRate, 1e-9)
	assert.InDelta(t, 0, result.CommissionFixedPerLot, 1e-9)
	assert.InDelta(t, (0.5/100.0)*price*100, result.CommissionPerLot, 1e-6)
}

func floatPtr(v float64) *float64 {
	return &v
}

func TestFixedRiskEngine_ComputeLot_StopMissing(t *testing.T) {
	engine := NewFixedRiskEngine(&stubSpecProvider{}, &stubQuoteProvider{}, &stubAccountState{}, &stubSymbolProvider{}, &stubGuard{}, Config{}, nil, nil)
	intent := &pb.TradeIntent{}
	policy := &domain.FixedRiskConfig{Amount: 100, Currency: "USD"}

	_, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	assert.Error(t, err)
}

func TestFixedRiskEngine_ComputeLot_TickValueMissing(t *testing.T) {
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": buildSpec(2, 0),
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(4000, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01},
		}},
		&stubGuard{},
		Config{RejectOnMissingTickValue: true},
		nil,
		nil,
	)
	price := 1000.0
	stop := 900.0
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop}
	policy := &domain.FixedRiskConfig{Amount: 50, Currency: "USD"}

	_, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	assert.Error(t, err)
}

func TestFixedRiskEngine_ComputeLot_CurrencyMismatch(t *testing.T) {
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": buildSpec(2, 1.0),
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(4000, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "EUR"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01},
		}},
		&stubGuard{},
		Config{RejectOnMissingTickValue: true},
		nil,
		nil,
	)
	price := 1000.0
	stop := 900.0
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop}
	policy := &domain.FixedRiskConfig{Amount: 50, Currency: "USD"}

	result, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	assert.Error(t, err)
	assert.Equal(t, DecisionReject, result.Decision)
}

func TestFixedRiskEngine_ComputeLot_QuoteStale(t *testing.T) {
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": buildSpec(2, 1.0),
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(4000, time.Now().Add(-2*time.Second)),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01},
		}},
		&stubGuard{},
		Config{MaxQuoteAge: 500 * time.Millisecond, RejectOnMissingTickValue: true},
		nil,
		nil,
	)
	price := 1000.0
	stop := 900.0
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop}
	policy := &domain.FixedRiskConfig{Amount: 50, Currency: "USD"}

	_, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	assert.Error(t, err)
}

func TestFixedRiskEngine_ComputeLot_RiskDriftExceeded(t *testing.T) {
	guard := &stubGuard{lot: 0.5, decision: volumeguard.DecisionPassThrough}
	engine := NewFixedRiskEngine(
		&stubSpecProvider{Specs: map[string]*pb.SymbolSpecification{
			"acc::XAUUSD": buildSpec(2, 1.0),
		}},
		&stubQuoteProvider{Quotes: map[string]*pb.SymbolQuoteSnapshot{
			"acc::XAUUSD": buildQuote(4000, time.Now()),
		}},
		&stubAccountState{Accounts: map[string]*pb.AccountInfo{
			"acc": {AccountId: "acc", Currency: "USD"},
		}},
		&stubSymbolProvider{Infos: map[string]*domain.AccountSymbolInfo{
			"acc::XAUUSD": {TickSize: 0.01},
		}},
		guard,
		Config{MaxRiskDrift: 0.02, RejectOnMissingTickValue: true},
		nil,
		nil,
	)
	price := 1000.0
	stop := 900.0
	intent := &pb.TradeIntent{Price: price, StopLoss: &stop}
	policy := &domain.FixedRiskConfig{Amount: 50, Currency: "USD"}

	result, err := engine.ComputeLot(context.Background(), "acc", "strategy", "XAUUSD", intent, policy)
	assert.Error(t, err)
	assert.Equal(t, DecisionReject, result.Decision)
	assert.Equal(t, "risk_drift_exceeded", result.Reason)
	assert.Zero(t, result.ExpectedLoss)
}
