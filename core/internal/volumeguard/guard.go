package volumeguard

import (
	"context"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// Decision describe la acción tomada por el guardián de volumen.
type Decision string

const (
	DecisionPassThrough Decision = "pass_through"
	DecisionClamp       Decision = "clamp"
	DecisionReject      Decision = "reject"
)

// SpecProvider expone el acceso a especificaciones de volumen.
type SpecProvider interface {
	GetVolumeSpec(ctx context.Context, accountID, canonical string) (*pb.VolumeSpec, int64, bool)
}

// Guard define el comportamiento del guardián de volumen.
type Guard interface {
	Execute(ctx context.Context, accountID, canonicalSymbol, strategyID string, requestedLot float64) (float64, Decision, error)
}

type guard struct {
	specs     SpecProvider
	policy    *domain.VolumeGuardPolicy
	telemetry *telemetry.Client
	metrics   *metricbundle.EchoMetrics
	clock     func() time.Time
}

// New crea un guardián de volumen.
func New(specs SpecProvider, policy *domain.VolumeGuardPolicy, tel *telemetry.Client, metrics *metricbundle.EchoMetrics) Guard {
	clk := time.Now
	return &guard{
		specs:     specs,
		policy:    policy,
		telemetry: tel,
		metrics:   metrics,
		clock:     clk,
	}
}

func (g *guard) Execute(ctx context.Context, accountID, canonicalSymbol, strategyID string, requestedLot float64) (float64, Decision, error) {
	attrs := []attribute.KeyValue{
		semconv.Echo.AccountID.String(accountID),
		semconv.Echo.Symbol.String(canonicalSymbol),
		semconv.Echo.Strategy.String(strategyID),
	}

	spec, reportedAtMs, found := g.specs.GetVolumeSpec(ctx, accountID, canonicalSymbol)
	if !found || spec == nil {
		g.recordDecision(ctx, DecisionReject, append(attrs, attribute.String("reason", "spec_missing"))...)
		return 0, DecisionReject, domain.NewError(domain.ErrSpecMissing, "volume spec missing")
	}

	nowMs := g.clock().UnixMilli()
	ageMs := float64(nowMs - reportedAtMs)
	if ageMs < 0 {
		ageMs = 0
	}
	g.recordSpecAge(ctx, ageMs, attrs...)

	if g.policy != nil && g.policy.MaxSpecAge > 0 {
		if time.Duration(ageMs)*time.Millisecond > g.policy.MaxSpecAge {
			g.recordDecision(ctx, DecisionReject, append(attrs,
				attribute.String("reason", "spec_stale"),
				attribute.Float64("spec_age_ms", ageMs),
			)...)
			return 0, DecisionReject, domain.NewError(domain.ErrSpecMissing, "volume spec stale")
		}
	}

	lot := requestedLot
	if lot <= 0 && g.policy != nil {
		lot = g.policy.DefaultLot
	}
	if lot <= 0 {
		lot = 0.01
	}

	clampedLot, clampErr := domain.ClampLotSize(spec, lot)
	decision := DecisionPassThrough
	reasonAttr := attribute.String("reason", "pass_through")

	if clampErr != nil {
		if validationErr, ok := clampErr.(*domain.ValidationError); ok {
			decision = DecisionClamp
			reasonAttr = attribute.String("reason", validationErr.Message)
		} else {
			g.recordDecision(ctx, DecisionReject, append(attrs,
				attribute.String("reason", "invalid_spec"),
				attribute.String("error", clampErr.Error()),
			)...)
			return 0, DecisionReject, clampErr
		}
	}

	if clampedLot <= 0 {
		g.recordDecision(ctx, DecisionReject, append(attrs,
			attribute.String("reason", "clamp_result_invalid"),
			attribute.Float64("requested_lot", lot),
		)...)
		return 0, DecisionReject, domain.NewValidationError("lot_size", clampedLot, "clamped lot invalid")
	}

	if decision == DecisionClamp && g.metrics != nil {
		g.metrics.RecordVolumeClamp(ctx, append(attrs,
			attribute.Float64("requested_lot", lot),
			attribute.Float64("final_lot", clampedLot),
		)...)
	}

	decisionAttrs := append(attrs,
		reasonAttr,
		attribute.Float64("requested_lot", lot),
		attribute.Float64("final_lot", clampedLot),
	)
	g.recordDecision(ctx, decision, decisionAttrs...)

	return clampedLot, decision, nil
}

func (g *guard) recordSpecAge(ctx context.Context, ageMs float64, attrs ...attribute.KeyValue) {
	if g.metrics == nil {
		return
	}
	g.metrics.RecordVolumeGuardSpecAge(ctx, ageMs, attrs...)
}

func (g *guard) recordDecision(ctx context.Context, decision Decision, attrs ...attribute.KeyValue) {
	if g.metrics != nil {
		g.metrics.RecordVolumeGuardDecision(ctx, string(decision), attrs...)
	}
	if g.telemetry != nil {
		switch decision {
		case DecisionReject:
			g.telemetry.Warn(ctx, "Volume guard rejected lot", attrs...)
		case DecisionClamp:
			g.telemetry.Info(ctx, "Volume guard clamped lot", attrs...)
		default:
			g.telemetry.Debug(ctx, "Volume guard passthrough", attrs...)
		}
	}
}
