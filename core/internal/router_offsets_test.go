package internal

import (
	"context"
	"math"
	"testing"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"google.golang.org/protobuf/proto"
)

func TestNormalizeSegment(t *testing.T) {
	cases := map[string]string{
		"tier_1":  "tier_1",
		"TIER_2":  "tier_2",
		"Tier3":   "tier_3",
		"GLOBAL":  "global",
		"":        "global",
		"unknown": "global",
	}

	for input, want := range cases {
		if got := normalizeSegment(input); got != want {
			t.Fatalf("normalizeSegment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNewSymbolPricingContext(t *testing.T) {
	info := &domain.AccountSymbolInfo{
		Digits:    5,
		Point:     0.00001,
		TickSize:  0.00001,
		StopLevel: 25,
	}

	ctx := newSymbolPricingContext(info, nil)
	if ctx.PipSize != 0.0001 {
		t.Fatalf("expected pip size 0.0001, got %f", ctx.PipSize)
	}
	expectedMin := float64(info.StopLevel) * info.Point
	if ctx.MinDistance != expectedMin {
		t.Fatalf("expected min distance %f, got %f", expectedMin, ctx.MinDistance)
	}
	if ctx.StopLevelPips <= 0 {
		t.Fatalf("expected stop level in pips to be > 0")
	}
}

func TestForceMinDistanceStops(t *testing.T) {
	intent := &pb.TradeIntent{
		Side:       pb.OrderSide_ORDER_SIDE_BUY,
		Price:      1.20000,
		StopLoss:   proto.Float64(1.19900),
		TakeProfit: proto.Float64(1.21000),
	}

	order := &pb.ExecuteOrder{Side: pb.OrderSide_ORDER_SIDE_BUY}
	pricing := symbolPricingContext{
		MinDistance: 0.0010,
		Digits:      5,
	}
	quote := &pb.SymbolQuoteSnapshot{Ask: 1.20100, Bid: 1.20050}

	forceMinDistanceStops(order, intent, pricing, quote)
	expectedSL := 1.20100 - 0.0010
	if order.StopLoss == nil || *order.StopLoss != expectedSL {
		t.Fatalf("expected stop loss %f, got %v", expectedSL, order.StopLoss)
	}

	intent.Side = pb.OrderSide_ORDER_SIDE_SELL
	order.Side = pb.OrderSide_ORDER_SIDE_SELL
	forceMinDistanceStops(order, intent, pricing, quote)
	expectedSL = 1.20050 + 0.0010
	if order.StopLoss == nil || *order.StopLoss != expectedSL {
		t.Fatalf("expected stop loss %f, got %v", expectedSL, order.StopLoss)
	}
}

func TestClassifyStopOffsetResult(t *testing.T) {
	if res := classifyStopOffsetResult(0, true, "applied", false); res != "skipped" {
		t.Fatalf("expected skipped for zero config, got %s", res)
	}
	if res := classifyStopOffsetResult(10, true, "applied", true); res != "clamped" {
		t.Fatalf("expected clamped when flagged, got %s", res)
	}
	if res := classifyStopOffsetResult(10, true, "applied", false); res != "applied" {
		t.Fatalf("expected applied, got %s", res)
	}
}

func TestComputeStopOffsetTargets_BuyOffsets(t *testing.T) {
	intent := &pb.TradeIntent{
		Side:       pb.OrderSide_ORDER_SIDE_BUY,
		Price:      1.20000,
		StopLoss:   proto.Float64(1.19900),
		TakeProfit: proto.Float64(1.20200),
	}
	policy := &domain.RiskPolicy{
		StopLossOffsetPips:   10,
		TakeProfitOffsetPips: -5,
	}
	pricing := symbolPricingContext{
		PipSize:       0.0001,
		StopLevelPips: 5,
	}

	stats, slTarget, tpTarget := computeStopOffsetTargets(intent, policy, pricing, "tier_1")
	if math.Abs(stats.TargetSLDistancePips-20) > 1e-9 {
		t.Fatalf("expected target SL distance 20 pips, got %f", stats.TargetSLDistancePips)
	}
	if math.Abs(stats.TargetTPDistancePips-15) > 1e-9 {
		t.Fatalf("expected target TP distance 15 pips, got %f", stats.TargetTPDistancePips)
	}

	if slTarget == nil {
		t.Fatalf("expected SL target to be set")
	}
	if math.Abs(*slTarget-1.19800) > 1e-9 {
		t.Fatalf("expected SL price 1.19800, got %f", *slTarget)
	}
	if stats.SLResult != "applied" {
		t.Fatalf("expected SL result applied, got %s", stats.SLResult)
	}

	if tpTarget == nil {
		t.Fatalf("expected TP target to be set")
	}
	if math.Abs(*tpTarget-1.20150) > 1e-9 {
		t.Fatalf("expected TP price 1.20150, got %f", *tpTarget)
	}
	if stats.TPResult != "applied" {
		t.Fatalf("expected TP result applied, got %s", stats.TPResult)
	}
}

func TestComputeStopOffsetTargets_ClampWhenOffsetTooSmall(t *testing.T) {
	intent := &pb.TradeIntent{
		Side:     pb.OrderSide_ORDER_SIDE_SELL,
		Price:    1.30000,
		StopLoss: proto.Float64(1.30100),
	}
	policy := &domain.RiskPolicy{
		StopLossOffsetPips: -50,
	}
	pricing := symbolPricingContext{
		PipSize:       0.0001,
		StopLevelPips: 5,
	}

	stats, slTarget, _ := computeStopOffsetTargets(intent, policy, pricing, "tier_2")
	if slTarget == nil {
		t.Fatalf("expected SL target to be clamped")
	}
	if stats.SLResult != "clamped" {
		t.Fatalf("expected SL result clamped, got %s", stats.SLResult)
	}
	if math.Abs(stats.TargetSLDistancePips-float64(pricing.StopLevelPips)) > 1e-9 {
		t.Fatalf("expected target SL distance equal to stop level, got %f", stats.TargetSLDistancePips)
	}
}

func TestRecordStopOffsetFallbackUpdatesContext(t *testing.T) {
	router := &Router{core: &Core{}}
	cmdCtx := &CommandContext{
		TradeID:        "trade-1",
		SlaveAccountID: "slave-1",
		Segment:        "tier_1",
	}

	router.recordStopOffsetFallback(context.Background(), cmdCtx, fallbackStageAttempt1, fallbackResultRequested, "cmd-original", "cmd-retry")

	if cmdCtx.LastFallbackStage != fallbackStageAttempt1 {
		t.Fatalf("expected stage %s, got %s", fallbackStageAttempt1, cmdCtx.LastFallbackStage)
	}
	if cmdCtx.LastFallbackResult != fallbackResultRequested {
		t.Fatalf("expected result %s, got %s", fallbackResultRequested, cmdCtx.LastFallbackResult)
	}
}
