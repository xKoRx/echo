package volumeguard

import (
	"context"
	"testing"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

type stubSpecProvider struct {
	spec       *pb.VolumeSpec
	reportedAt int64
	found      bool
}

func (s stubSpecProvider) GetVolumeSpec(ctx context.Context, accountID, canonical string) (*pb.VolumeSpec, int64, bool) {
	return s.spec, s.reportedAt, s.found
}

func TestGuardPassThrough(t *testing.T) {
	provider := stubSpecProvider{
		spec:       &pb.VolumeSpec{MinVolume: 0.1, MaxVolume: 10, VolumeStep: 0.1},
		reportedAt: time.Now().UnixMilli(),
		found:      true,
	}
	policy := &domain.VolumeGuardPolicy{
		OnMissingSpec:  domain.VolumeGuardMissingSpecReject,
		MaxSpecAge:     10 * time.Second,
		AlertThreshold: 8 * time.Second,
		DefaultLot:     0.5,
	}

	guard := New(provider, policy, nil, nil).(*guard)
	guard.clock = func() time.Time { return time.Now() }

	lot, decision, err := guard.Execute(context.Background(), "acc", "XAUUSD", "strategy", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != DecisionPassThrough {
		t.Fatalf("expected pass_through decision, got %s", decision)
	}
	if lot != 0.5 {
		t.Fatalf("expected lot 0.5, got %.2f", lot)
	}
}

func TestGuardClamp(t *testing.T) {
	provider := stubSpecProvider{
		spec:       &pb.VolumeSpec{MinVolume: 0.1, MaxVolume: 1, VolumeStep: 0.1},
		reportedAt: time.Now().UnixMilli(),
		found:      true,
	}
	policy := &domain.VolumeGuardPolicy{
		OnMissingSpec: domain.VolumeGuardMissingSpecReject,
		MaxSpecAge:    10 * time.Second,
		DefaultLot:    0.5,
	}

	guard := New(provider, policy, nil, nil).(*guard)
	guard.clock = func() time.Time { return time.Now() }

	lot, decision, err := guard.Execute(context.Background(), "acc", "XAUUSD", "strategy", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != DecisionClamp {
		t.Fatalf("expected clamp decision, got %s", decision)
	}
	if lot != 1 {
		t.Fatalf("expected lot 1, got %.2f", lot)
	}
}

func TestGuardRejectMissingSpec(t *testing.T) {
	provider := stubSpecProvider{found: false}
	policy := &domain.VolumeGuardPolicy{
		OnMissingSpec: domain.VolumeGuardMissingSpecReject,
		MaxSpecAge:    10 * time.Second,
		DefaultLot:    0.2,
	}

	guard := New(provider, policy, nil, nil).(*guard)
	guard.clock = func() time.Time { return time.Now() }

	_, decision, err := guard.Execute(context.Background(), "acc", "XAUUSD", "strategy", 0.2)
	if decision != DecisionReject {
		t.Fatalf("expected reject decision, got %s", decision)
	}
	if err == nil {
		t.Fatalf("expected error for missing spec")
	}
	if tradingErr, ok := err.(*domain.TradingError); !ok || tradingErr.Code != domain.ErrSpecMissing {
		t.Fatalf("expected ErrSpecMissing, got %v", err)
	}
}

func TestGuardRejectStaleSpec(t *testing.T) {
	provider := stubSpecProvider{
		spec:       &pb.VolumeSpec{MinVolume: 0.1, MaxVolume: 10, VolumeStep: 0.1},
		reportedAt: time.Now().Add(-2 * time.Minute).UnixMilli(),
		found:      true,
	}
	policy := &domain.VolumeGuardPolicy{
		OnMissingSpec: domain.VolumeGuardMissingSpecReject,
		MaxSpecAge:    30 * time.Second,
		DefaultLot:    0.3,
	}

	guard := New(provider, policy, nil, nil).(*guard)
	guard.clock = func() time.Time { return time.Now() }

	_, decision, err := guard.Execute(context.Background(), "acc", "XAUUSD", "strategy", 0.3)
	if decision != DecisionReject {
		t.Fatalf("expected reject decision for stale spec, got %s", decision)
	}
	if err == nil {
		t.Fatalf("expected error for stale spec")
	}
}
