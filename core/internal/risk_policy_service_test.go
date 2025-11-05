package internal

import (
	"context"
	"testing"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
)

type stubRiskPolicyRepo struct {
	policy  *domain.RiskPolicy
	err     error
	fetches int
}

func (s *stubRiskPolicyRepo) Get(ctx context.Context, accountID, strategyID string) (*domain.RiskPolicy, error) {
	s.fetches++
	if s.err != nil {
		return nil, s.err
	}
	if s.policy == nil {
		return nil, nil
	}
	return s.policy, nil
}

func TestRiskPolicyServiceCache(t *testing.T) {
	repo := &stubRiskPolicyRepo{policy: &domain.RiskPolicy{
		AccountID:  "acc",
		StrategyID: "strat",
		Type:       domain.RiskPolicyTypeFixedLot,
		FixedLot:   &domain.FixedLotConfig{LotSize: 0.5},
	}}
	svc := NewRiskPolicyService(repo, time.Minute, nil, nil)
	ctx := context.Background()

	policy1, err := svc.Get(ctx, "acc", "strat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy1 == nil {
		t.Fatalf("expected policy, got nil")
	}
	if repo.fetches != 1 {
		t.Fatalf("expected 1 fetch, got %d", repo.fetches)
	}

	// Cached response should not hit repository again
	if _, err := svc.Get(ctx, "acc", "strat"); err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}
	if repo.fetches != 1 {
		t.Fatalf("expected cache hit without repo fetch, got %d fetches", repo.fetches)
	}

	// Invalidate and ensure repository is consulted again
	svc.Invalidate("acc", "strat")
	if _, err := svc.Get(ctx, "acc", "strat"); err != nil {
		t.Fatalf("unexpected error after invalidate: %v", err)
	}
	if repo.fetches != 2 {
		t.Fatalf("expected second fetch after invalidate, got %d", repo.fetches)
	}
}

func TestRiskPolicyServiceInvalidateAccount(t *testing.T) {
	repo := &stubRiskPolicyRepo{}
	svc := NewRiskPolicyService(repo, time.Minute, nil, nil)
	ctx := context.Background()

	// Seed cache with two strategies
	repo.policy = &domain.RiskPolicy{AccountID: "acc", StrategyID: "A", Type: domain.RiskPolicyTypeFixedLot, FixedLot: &domain.FixedLotConfig{LotSize: 0.1}}
	if _, err := svc.Get(ctx, "acc", "A"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	repo.policy = &domain.RiskPolicy{AccountID: "acc", StrategyID: "B", Type: domain.RiskPolicyTypeFixedLot, FixedLot: &domain.FixedLotConfig{LotSize: 0.2}}
	if _, err := svc.Get(ctx, "acc", "B"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalidate account (wildcard)
	svc.Invalidate("acc", "")
	repo.policy = &domain.RiskPolicy{AccountID: "acc", StrategyID: "A", Type: domain.RiskPolicyTypeFixedLot, FixedLot: &domain.FixedLotConfig{LotSize: 0.3}}
	if _, err := svc.Get(ctx, "acc", "A"); err != nil {
		t.Fatalf("unexpected error after account invalidate: %v", err)
	}
	if repo.fetches != 3 {
		t.Fatalf("expected fetch after account invalidate, got %d", repo.fetches)
	}
}

// Ensure interface compliance during tests
var _ domain.RiskPolicyRepository = (*stubRiskPolicyRepo)(nil)
