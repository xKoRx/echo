package internal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/xKoRx/echo/core/capabilities"
	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

var errMissingIntent = errors.New("stop level guard: trade intent is required")

// stopLevelGuard implementa capabilities.StopLevelGuard.
type stopLevelGuard struct {
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics
}

// NewStopLevelGuard crea una instancia del guardián de StopLevel.
func NewStopLevelGuard(tel *telemetry.Client, metrics *metricbundle.EchoMetrics) capabilities.StopLevelGuard {
	return &stopLevelGuard{
		telemetry:   tel,
		echoMetrics: metrics,
	}
}

// Evaluate aplica la lógica determinada en el RFC i8(a+b) para offsets y StopLevel.
func (g *stopLevelGuard) Evaluate(ctx context.Context, req *capabilities.StopLevelGuardRequest) (*capabilities.StopLevelGuardResult, error) {
	start := time.Now()
	ctx, span := g.telemetry.StartSpan(ctx, "core.stop_level.guard")
	defer span.End()

	if req == nil || req.Intent == nil {
		return nil, errMissingIntent
	}

	intent := req.Intent
	adjustable := ensureAdjustableStops(req.AdjustableStops)

	point := resolvePoint(req.SymbolInfo, req.SymbolSpec)
	entryPrice := resolveEntryPrice(intent, req.Quote)
	stopLevelPoints := resolveStopLevelPoints(req.SymbolInfo, req.SymbolSpec)

	attrs := []attribute.KeyValue{
		semconv.Echo.AccountID.String(req.AccountID),
		semconv.Echo.Symbol.String(intent.Symbol),
		attribute.Float64("point", point),
		attribute.Float64("stop_level_points", stopLevelPoints),
		attribute.Float64("entry_price", entryPrice),
	}

	if point <= 0 {
		err := fmt.Errorf("stop level guard: missing point configuration for symbol %s", intent.Symbol)
		g.telemetry.Error(ctx, "StopLevelGuard missing point", err, attrs...)
		return nil, err
	}
	if entryPrice <= 0 {
		err := fmt.Errorf("stop level guard: invalid entry price for symbol %s", intent.Symbol)
		g.telemetry.Error(ctx, "StopLevelGuard invalid entry price", err, attrs...)
		return nil, err
	}

	slRequired := isStopLossRequired(intent, req.Policy)
	tpRequired := isTakeProfitRequired(intent, req.Policy)

	slGapPts, slEffective, slAppliedPts, slNeedsPostModify, slValid := evaluateLevel(intent.Side, intent.StopLoss, adjustable.SLOffsetPoints, point, entryPrice, stopLevelPoints, slRequired)
	tpGapPts, tpEffective, tpAppliedPts, tpNeedsPostModify, tpValid := evaluateLevel(intent.Side, intent.TakeProfit, adjustable.TPOffsetPoints, point, entryPrice, stopLevelPoints, tpRequired)

	decision := capabilities.StopLevelDecisionAcceptWithOffsets
	reason := "offset_applied"

	if !slValid {
		reason = "stop_level_pre_fill_sl"
		decision = capabilities.StopLevelDecisionRejectWithReason
	} else if !tpValid {
		reason = "stop_level_pre_fill_tp"
		decision = capabilities.StopLevelDecisionRejectWithReason
	} else if slNeedsPostModify || tpNeedsPostModify {
		reason = "stop_level_post_modify"
		decision = capabilities.StopLevelDecisionAcceptWithPostModify
	}

	stopLevelBreach := decision == capabilities.StopLevelDecisionAcceptWithPostModify
	adjustable.StopLevelBreach = stopLevelBreach
	if stopLevelBreach {
		adjustable.Reason = fmt.Sprintf("%s|safe_sl=%.10f|target_sl=%.10f|safe_tp=%.10f|target_tp=%.10f|applied_sl_points=%d|applied_tp_points=%d",
			reason,
			result.SafeStopLossPrice,
			result.TargetStopLossPrice,
			result.SafeTakeProfitPrice,
			result.TargetTakeProfitPrice,
			result.AppliedSLOffsetPts,
			result.AppliedTPOffsetPts,
		)
	} else {
		adjustable.Reason = reason
	}

	result := &capabilities.StopLevelGuardResult{
		Decision:           decision,
		Reason:             reason,
		AdjustableStops:    adjustable,
		StopLevelBreach:    stopLevelBreach,
		EffectiveSLPoints:  slEffective,
		EffectiveTPPoints:  tpEffective,
		AppliedSLOffsetPts: slAppliedPts,
		AppliedTPOffsetPts: tpAppliedPts,
		EntryPrice:         entryPrice,
	}

	if intent.StopLoss != nil && *intent.StopLoss != 0 && point > 0 {
		safeDistance := float64(slAppliedPts) * point
		targetDistance := float64(adjustable.SLOffsetPoints) * point
		if safeDistance > 0 {
			result.SafeStopLossPrice = computeStopPrice(intent.Side, entryPrice, safeDistance)
		}
		if targetDistance > 0 {
			result.TargetStopLossPrice = computeStopPrice(intent.Side, entryPrice, targetDistance)
		}
	}

	if intent.TakeProfit != nil && *intent.TakeProfit != 0 && point > 0 {
		safeDistance := float64(tpAppliedPts) * point
		targetDistance := float64(adjustable.TPOffsetPoints) * point
		if safeDistance > 0 {
			result.SafeTakeProfitPrice = computeTakeProfitPrice(intent.Side, entryPrice, safeDistance)
		}
		if targetDistance > 0 {
			result.TargetTakeProfitPrice = computeTakeProfitPrice(intent.Side, entryPrice, targetDistance)
		}
	}

	attrs = append(attrs,
		attribute.String("decision", string(decision)),
		attribute.Bool("stop_level_breach", stopLevelBreach),
		attribute.Float64("sl_gap_points", slGapPts),
		attribute.Float64("tp_gap_points", tpGapPts),
		attribute.Float64("effective_sl_points", slEffective),
		attribute.Float64("effective_tp_points", tpEffective),
		attribute.Int("configured_sl_offset_points", int(adjustable.SLOffsetPoints)),
		attribute.Int("configured_tp_offset_points", int(adjustable.TPOffsetPoints)),
		attribute.Int("applied_sl_offset_points", int(slAppliedPts)),
		attribute.Int("applied_tp_offset_points", int(tpAppliedPts)),
	)

	ctx = telemetry.AppendEventAttrs(ctx, attrs...)

	switch decision {
	case capabilities.StopLevelDecisionRejectWithReason:
		g.telemetry.Warn(ctx, "StopLevelGuard rejection", attrs...)
		g.recordStopLevelRejection(ctx, attrs...)
	case capabilities.StopLevelDecisionAcceptWithPostModify:
		g.telemetry.Info(ctx, "StopLevelGuard accept with post modify", attrs...)
	default:
		g.telemetry.Info(ctx, "StopLevelGuard accept", attrs...)
	}

	g.recordOffsetDuration(ctx, time.Since(start), attrs...)

	return result, nil
}

func (g *stopLevelGuard) recordStopLevelRejection(ctx context.Context, attrs ...attribute.KeyValue) {
	if g.echoMetrics == nil {
		return
	}
	g.echoMetrics.RecordStopLevelRejection(ctx, attrs...)
}

func (g *stopLevelGuard) recordOffsetDuration(ctx context.Context, d time.Duration, attrs ...attribute.KeyValue) {
	if g.echoMetrics == nil {
		return
	}
	g.echoMetrics.RecordOffsetApplyDuration(ctx, float64(d.Milliseconds()), attrs...)
}

func ensureAdjustableStops(stops *domain.AdjustableStops) *domain.AdjustableStops {
	if stops != nil {
		copy := *stops
		return &copy
	}
	return &domain.AdjustableStops{}
}

func resolvePoint(info *domain.AccountSymbolInfo, spec *pb.SymbolSpecification) float64 {
	if info != nil && info.Point > 0 {
		return info.Point
	}
	if spec != nil && spec.General != nil {
		if spec.General.Point > 0 {
			return spec.General.Point
		}
		if spec.General.Digits > 0 {
			return math.Pow10(-int(spec.General.Digits))
		}
	}
	return 0
}

func resolveStopLevelPoints(info *domain.AccountSymbolInfo, spec *pb.SymbolSpecification) float64 {
	if info != nil && info.StopLevel > 0 {
		return float64(info.StopLevel)
	}
	if spec != nil && spec.General != nil && spec.General.StopsLevel > 0 {
		return float64(spec.General.StopsLevel)
	}
	return 0
}

func resolveEntryPrice(intent *pb.TradeIntent, quote *pb.SymbolQuoteSnapshot) float64 {
	if quote != nil {
		if intent.Side == pb.OrderSide_ORDER_SIDE_BUY && quote.Ask > 0 {
			return quote.Ask
		}
		if intent.Side == pb.OrderSide_ORDER_SIDE_SELL && quote.Bid > 0 {
			return quote.Bid
		}
	}
	// Fallback a precio del intent (i1 comportamiento histórico)
	return intent.Price
}

func evaluateLevel(side pb.OrderSide, level *float64, configured int32, point, entry, stopLevel float64, required bool) (gapPoints float64, effective float64, applied int32, needsPostModify bool, valid bool) {
	if level == nil || *level == 0 {
		if required {
			return 0, 0, configured, false, false
		}
		return 0, math.Inf(1), configured, false, true
	}
	if point <= 0 || entry <= 0 {
		return 0, 0, configured, false, false
	}

	distance := computeDistance(side, entry, *level)
	if distance <= 0 {
		return 0, 0, configured, false, false
	}

	gapPoints = distance / point
	if gapPoints <= 0 {
		return gapPoints, 0, configured, false, false
	}

	applied = configured
	if applied < 0 {
		applied = 0
	}

	effective = gapPoints - float64(applied)
	margin := 1e-6

	if stopLevel > 0 && effective <= stopLevel {
		needed := stopLevel - effective + margin
		if needed < 0 {
			needed = 0
		}
		extra := int32(math.Ceil(needed))
		if extra > 0 {
			applied += extra
			effective = gapPoints - float64(applied)
		}
	}

	if float64(applied) >= gapPoints {
		return gapPoints, effective, applied, false, false
	}

	if stopLevel > 0 && effective <= stopLevel {
		return gapPoints, effective, applied, false, false
	}

	needsPostModify = !required && applied > configured

	return gapPoints, effective, applied, needsPostModify, true
}

func computeDistance(side pb.OrderSide, entry, level float64) float64 {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		return entry - level
	case pb.OrderSide_ORDER_SIDE_SELL:
		return level - entry
	default:
		return 0
	}
}

func isStopLossRequired(intent *pb.TradeIntent, _ *domain.RiskPolicy) bool {
	return intent.StopLoss != nil && *intent.StopLoss != 0
}

func isTakeProfitRequired(_ *pb.TradeIntent, _ *domain.RiskPolicy) bool {
	return false
}
