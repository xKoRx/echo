package internal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

type stubRiskPolicyService struct {
	policy *domain.RiskPolicy
	err    error
}

func (s *stubRiskPolicyService) Get(ctx context.Context, accountID, strategyID string) (*domain.RiskPolicy, error) {
	return s.policy, s.err
}

func (s *stubRiskPolicyService) GetAdjustableStops(ctx context.Context, accountID, symbol string) (*domain.AdjustableStops, error) {
	return &domain.AdjustableStops{}, nil
}

func (s *stubRiskPolicyService) Invalidate(accountID, strategyID string) {}

type stubHandshakeRepo struct {
	mu          sync.Mutex
	evaluations []*handshake.Evaluation
}

func (s *stubHandshakeRepo) CreateEvaluation(ctx context.Context, evaluation *handshake.Evaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned := cloneEvaluation(evaluation)
	s.evaluations = append(s.evaluations, cloned)
	return nil
}

func (s *stubHandshakeRepo) GetLatestByAccount(ctx context.Context, accountID string) (*handshake.Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.evaluations) - 1; i >= 0; i-- {
		if s.evaluations[i].AccountID == accountID {
			return cloneEvaluation(s.evaluations[i]), nil
		}
	}
	return nil, nil
}

func cloneEvaluation(e *handshake.Evaluation) *handshake.Evaluation {
	if e == nil {
		return nil
	}
	cloned := *e
	if len(e.Errors) > 0 {
		cloned.Errors = append([]handshake.Issue(nil), e.Errors...)
	}
	if len(e.Warnings) > 0 {
		cloned.Warnings = append([]handshake.Issue(nil), e.Warnings...)
	}
	if len(e.Entries) > 0 {
		cloned.Entries = make([]handshake.Entry, len(e.Entries))
		copy(cloned.Entries, e.Entries)
	}
	if len(e.RequiredFeatures) > 0 {
		cloned.RequiredFeatures = append([]string(nil), e.RequiredFeatures...)
	}
	if len(e.OptionalFeatures) > 0 {
		cloned.OptionalFeatures = append([]string(nil), e.OptionalFeatures...)
	}
	cloned.Capabilities = handshake.CapabilitySet{
		Features: append([]string(nil), e.Capabilities.Features...),
		Metrics:  append([]string(nil), e.Capabilities.Metrics...),
	}
	return &cloned
}

func TestHandshakeEvaluator_PersistWhenChanged(t *testing.T) {
	accountID := "12345"
	canonical := "XAUUSD"

	repo := &stubHandshakeRepo{}
	riskSvc := &stubRiskPolicyService{
		policy: &domain.RiskPolicy{
			AccountID:  accountID,
			StrategyID: "default",
			Type:       domain.RiskPolicyTypeFixedLot,
			FixedLot:   &domain.FixedLotConfig{LotSize: 0.1},
		},
	}

	canonicalValidator := NewCanonicalValidator([]string{canonical}, UnknownActionReject, nil, nil)

	specService := &SymbolSpecService{
		specs: map[string]map[string]*specCacheEntry{
			accountID: {
				canonical: {
					spec:         &pb.SymbolSpecification{CanonicalSymbol: canonical},
					reportedAtMs: time.Now().Add(-2 * time.Second).UnixMilli(),
				},
			},
		},
	}

	versionRange, err := handshake.NewVersionRange(handshake.ProtocolVersionV2, handshake.ProtocolVersionV2, nil)
	require.NoError(t, err)

	evaluator := NewHandshakeEvaluator(
		NewProtocolValidator(&ProtocolConfig{VersionRange: versionRange}),
		canonicalValidator,
		specService,
		riskSvc,
		repo,
		nil,
		nil,
		&ProtocolConfig{VersionRange: versionRange},
		5*time.Second,
	)

	metadata := &pb.HandshakeMetadata{
		ProtocolVersion:  int32(handshake.ProtocolVersionV2),
		ClientSemver:     "1.0.0",
		RequiredFeatures: []string{"spec_report/v1"},
	}

	mappings := []*domain.SymbolMapping{{
		CanonicalSymbol: canonical,
		BrokerSymbol:    "XAUUSD",
	}}

	evaluation, persisted, err := evaluator.Evaluate(context.Background(), accountID, "agent-1", "slave", "core-1", metadata, mappings)
	require.NoError(t, err)
	require.True(t, persisted)
	require.NotNil(t, evaluation)
	assert.Equal(t, handshake.RegistrationStatusAccepted, evaluation.Status)
	assert.Len(t, repo.evaluations, 1)
}

func TestHandshakeEvaluator_SkipWhenEquivalent(t *testing.T) {
	accountID := "99999"
	canonical := "XAUUSD"

	repo := &stubHandshakeRepo{}
	riskSvc := &stubRiskPolicyService{
		policy: &domain.RiskPolicy{
			AccountID:  accountID,
			StrategyID: "default",
			Type:       domain.RiskPolicyTypeFixedLot,
			FixedLot:   &domain.FixedLotConfig{LotSize: 0.2},
		},
	}

	canonicalValidator := NewCanonicalValidator([]string{canonical}, UnknownActionReject, nil, nil)

	specService := &SymbolSpecService{
		specs: map[string]map[string]*specCacheEntry{
			accountID: {
				canonical: {
					spec:         &pb.SymbolSpecification{CanonicalSymbol: canonical},
					reportedAtMs: time.Now().Add(-3 * time.Second).UnixMilli(),
				},
			},
		},
	}

	versionRange, err := handshake.NewVersionRange(handshake.ProtocolVersionV2, handshake.ProtocolVersionV2, nil)
	require.NoError(t, err)

	evaluator := NewHandshakeEvaluator(
		NewProtocolValidator(&ProtocolConfig{VersionRange: versionRange}),
		canonicalValidator,
		specService,
		riskSvc,
		repo,
		nil,
		nil,
		&ProtocolConfig{VersionRange: versionRange},
		5*time.Second,
	)

	metadata := &pb.HandshakeMetadata{
		ProtocolVersion:  int32(handshake.ProtocolVersionV2),
		ClientSemver:     "1.0.0",
		RequiredFeatures: []string{"spec_report/v1"},
	}

	mappings := []*domain.SymbolMapping{{
		CanonicalSymbol: canonical,
		BrokerSymbol:    "XAUUSD",
	}}

	firstEval, persisted, err := evaluator.Evaluate(context.Background(), accountID, "agent-2", "slave", "core-1", metadata, mappings)
	require.NoError(t, err)
	require.True(t, persisted)
	require.NotNil(t, firstEval)
	require.Len(t, repo.evaluations, 1)

	secondEval, persistedSecond, err := evaluator.Evaluate(context.Background(), accountID, "agent-2", "slave", "core-1", metadata, mappings)
	require.NoError(t, err)
	require.False(t, persistedSecond)
	require.NotNil(t, secondEval)
	assert.Len(t, repo.evaluations, 1)
	assert.Equal(t, repo.evaluations[0].EvaluationID, secondEval.EvaluationID)
}
