package riskengine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/xKoRx/echo/core/internal/volumeguard"
	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Decision representa la acción tomada por el motor FixedRisk.
type Decision string

const (
	DecisionUnknown Decision = "unknown"
	DecisionProceed Decision = "proceed"
	DecisionReject  Decision = "reject"
	DecisionDefer   Decision = "defer"
)

// Config contiene los parámetros del motor FixedRisk.
type Config struct {
	MaxQuoteAge              time.Duration
	MinDistancePoints        float64
	MaxRiskDrift             float64
	DefaultCurrency          string
	EnableCurrencyFallback   bool
	RejectOnMissingTickValue bool
}

// Result encapsula el resultado de un cálculo de riesgo fijo.
type Result struct {
	Lot                   float64
	ExpectedLoss          float64
	Decision              Decision
	Reason                string
	GuardDecision         volumeguard.Decision
	CommissionFixedPerLot float64
	CommissionPerLot      float64
	CommissionTotal       float64
	CommissionRate        float64
}

// AccountStateProvider expone la lectura de información de cuenta.
type AccountStateProvider interface {
	Get(accountID string) (*pb.AccountInfo, bool)
}

// SymbolSpecProvider expone la lectura de especificaciones de símbolos.
type SymbolSpecProvider interface {
	GetSpecification(ctx context.Context, accountID, canonical string) (*pb.SymbolSpecification, int64, bool)
}

// QuoteProvider expone la lectura de snapshots de precios.
type QuoteProvider interface {
	Get(accountID, canonical string) (*pb.SymbolQuoteSnapshot, bool)
}

// SymbolDetailsProvider expone metadatos del símbolo (tick size, etc.).
type SymbolDetailsProvider interface {
	Resolve(ctx context.Context, accountID, canonical string) (*domain.AccountSymbolInfo, bool)
}

// FixedRiskEngine implementa la lógica de cálculo de lote por riesgo monetario.
type FixedRiskEngine struct {
	specs     SymbolSpecProvider
	quotes    QuoteProvider
	accounts  AccountStateProvider
	symbols   SymbolDetailsProvider
	guard     volumeguard.Guard
	config    Config
	telemetry *telemetry.Client
	metrics   *metricbundle.EchoMetrics
}

// NewFixedRiskEngine crea una instancia del motor FixedRisk.
func NewFixedRiskEngine(specs SymbolSpecProvider, quotes QuoteProvider, accounts AccountStateProvider, symbols SymbolDetailsProvider, guard volumeguard.Guard, cfg Config, tel *telemetry.Client, metrics *metricbundle.EchoMetrics) *FixedRiskEngine {
	return &FixedRiskEngine{
		specs:     specs,
		quotes:    quotes,
		accounts:  accounts,
		symbols:   symbols,
		guard:     guard,
		config:    cfg,
		telemetry: tel,
		metrics:   metrics,
	}
}

// ComputeLot ejecuta el cálculo de riesgo fijo retornando el lote sugerido y métricas asociadas.
func (e *FixedRiskEngine) ComputeLot(ctx context.Context, accountID, strategyID, canonicalSymbol string, intent *pb.TradeIntent, policy *domain.FixedRiskConfig) (Result, error) {
	result := Result{Decision: DecisionReject, Reason: "unknown"}

	if intent == nil || policy == nil {
		return result, errors.New("intento o política inválida")
	}

	commissionFixedPerLot := 0.0
	if policy.CommissionPerLot != nil && *policy.CommissionPerLot > 0 {
		commissionFixedPerLot = *policy.CommissionPerLot
	}

	commissionRatePercent := 0.0
	commissionRate := 0.0
	if policy.CommissionRate != nil && *policy.CommissionRate > 0 {
		commissionRatePercent = *policy.CommissionRate
		commissionRate = commissionRatePercent / 100.0
	}

	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.PolicyType.String(string(domain.RiskPolicyTypeFixedRisk)),
		semconv.Echo.RiskAmount.Float64(policy.Amount),
		semconv.Echo.RiskCurrency.String(strings.ToUpper(policy.Currency)),
		attribute.Float64("commission_fixed_per_lot", commissionFixedPerLot),
		attribute.Float64("commission_rate_percent", commissionRatePercent),
		semconv.Echo.RiskCommissionRate.Float64(commissionRate),
	)
	ctx = telemetry.AppendMetricAttrs(ctx,
		semconv.Echo.PolicyType.String(string(domain.RiskPolicyTypeFixedRisk)),
		semconv.Echo.RiskAmount.Float64(policy.Amount),
		semconv.Echo.RiskCurrency.String(strings.ToUpper(policy.Currency)),
		attribute.Float64("commission_fixed_per_lot", commissionFixedPerLot),
		attribute.Float64("commission_rate_percent", commissionRatePercent),
		semconv.Echo.RiskCommissionRate.Float64(commissionRate),
	)

	var span trace.Span
	if e.telemetry != nil {
		ctx, span = e.telemetry.StartSpan(ctx, "core.risk.calculate")
		span.SetAttributes(
			semconv.Echo.AccountID.String(accountID),
			semconv.Echo.Strategy.String(strategyID),
			semconv.Echo.Symbol.String(canonicalSymbol),
		)
		defer span.End()
	}

	baseAttrs := []attribute.KeyValue{
		semconv.Echo.AccountID.String(accountID),
		semconv.Echo.Strategy.String(strategyID),
		semconv.Echo.Symbol.String(canonicalSymbol),
		attribute.Float64("risk_amount", policy.Amount),
		attribute.String("risk_currency", strings.ToUpper(policy.Currency)),
	}
	baseAttrs = append(baseAttrs,
		attribute.Float64("commission_fixed_per_lot", commissionFixedPerLot),
		attribute.Float64("commission_rate_percent", commissionRatePercent),
		semconv.Echo.RiskCommissionRate.Float64(commissionRate),
	)
	e.logInfo(ctx, "Fixed risk calculation started", baseAttrs...)

	if policy.Amount <= 0 {
		result.Reason = "invalid_policy_amount"
		e.recordError(ctx, domain.NewValidationError("amount", policy.Amount, "fixed risk amount must be positive"))
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, baseAttrs...)
		return result, domain.NewValidationError("amount", policy.Amount, "fixed risk amount must be positive")
	}

	if intent.StopLoss == nil {
		result.Reason = "stop_required"
		err := domain.NewError(domain.ErrStopRequired, "trade intent missing stop loss")
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, baseAttrs...)
		return result, err
	}

	spec, _, specFound := e.specs.GetSpecification(ctx, accountID, canonicalSymbol)
	if !specFound || spec == nil || spec.General == nil {
		result.Reason = "spec_missing"
		err := domain.NewError(domain.ErrSpecMissing, "symbol specification missing for fixed risk")
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, baseAttrs...)
		return result, err
	}

	general := spec.General
	tickSize := 0.0
	var symbolInfo *domain.AccountSymbolInfo
	if e.symbols != nil {
		if info, ok := e.symbols.Resolve(ctx, accountID, canonicalSymbol); ok && info != nil {
			symbolInfo = info
			if info.TickSize > 0 {
				tickSize = info.TickSize
			} else if info.Point > 0 {
				tickSize = info.Point
			}
		}
	}
	if tickSize <= 0 && spec.Volume != nil {
		if spec.Volume.VolumeStep > 0 {
			tickSize = spec.Volume.VolumeStep
		}
	}
	if tickSize <= 0 && general != nil && general.Digits > 0 {
		precision := math.Pow(10, -float64(general.Digits))
		if precision > 0 {
			tickSize = precision
		}
	}
	if tickSize <= 0 {
		result.Reason = "invalid_tick_size"
		err := domain.NewValidationError("tick_size", tickSize, "tick size must be positive")
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, append(baseAttrs, attribute.Float64("tick_size", tickSize))...)
		return result, err
	}

	tickValue := general.TickValue
	if tickValue <= 0 && e.config.RejectOnMissingTickValue {
		result.Reason = "tick_value_missing"
		err := domain.NewValidationError("tick_value", tickValue, "tick_value must be > 0 for fixed risk calculations")
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, baseAttrs...)
		return result, err
	}

	contractSize := 1.0
	if general != nil && general.ContractSize > 0 {
		contractSize = general.ContractSize
	} else if symbolInfo != nil && symbolInfo.ContractSize != nil && *symbolInfo.ContractSize > 0 {
		contractSize = *symbolInfo.ContractSize
	}
	if contractSize <= 0 {
		contractSize = 1
	}

	baseAttrs = append(baseAttrs, attribute.Float64("contract_size", contractSize))

	quote, quoteFound := e.quotes.Get(accountID, canonicalSymbol)
	if !quoteFound || quote == nil {
		result.Reason = "quote_missing"
		err := domain.NewError(domain.ErrSpecMissing, "quote snapshot missing for fixed risk")
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, baseAttrs...)
		return result, err
	}

	if e.config.MaxQuoteAge > 0 {
		age := time.Since(time.UnixMilli(quote.TimestampMs))
		if age > e.config.MaxQuoteAge {
			result.Reason = "quote_stale"
			err := fmt.Errorf("quote age %s exceeds max quote age %s", age, e.config.MaxQuoteAge)
			e.recordError(ctx, err)
			e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
			e.logOutcome(ctx, result.Decision, result.Reason, append(baseAttrs, attribute.String("quote_age", age.String()))...)
			return result, err
		}
	}

	accountCurrency := strings.ToUpper(strings.TrimSpace(policy.Currency))
	if info, ok := e.accounts.Get(accountID); ok && info != nil {
		if info.Currency != "" {
			accountCurrency = strings.ToUpper(info.Currency)
		}
	} else if e.config.EnableCurrencyFallback && e.config.DefaultCurrency != "" {
		accountCurrency = strings.ToUpper(e.config.DefaultCurrency)
	} else if !ok {
		result.Reason = "account_state_missing"
		err := fmt.Errorf("account state missing for account %s", accountID)
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, baseAttrs...)
		return result, err
	}

	if !strings.EqualFold(accountCurrency, policy.Currency) {
		result.Reason = "currency_mismatch"
		err := fmt.Errorf("policy currency %s differs from account currency %s", policy.Currency, accountCurrency)
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, append(baseAttrs, attribute.String("account_currency", accountCurrency))...)
		return result, err
	}

	distancePrice := math.Abs(intent.GetPrice() - intent.GetStopLoss())
	distancePoints := distancePrice / tickSize
	if e.config.MinDistancePoints > 0 && distancePoints < e.config.MinDistancePoints {
		result.Reason = "distance_too_small"
		err := fmt.Errorf("distance points %.2f below minimum %.2f", distancePoints, e.config.MinDistancePoints)
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, append(baseAttrs, attribute.Float64("distance_points", distancePoints))...)
		return result, err
	}

	entryPrice := intent.GetPrice()
	if entryPrice <= 0 {
		if intent.Side == pb.OrderSide_ORDER_SIDE_SELL {
			if quote.Bid > 0 {
				entryPrice = quote.Bid
			} else if quote.Ask > 0 {
				entryPrice = quote.Ask
			}
		} else {
			if quote.Ask > 0 {
				entryPrice = quote.Ask
			} else if quote.Bid > 0 {
				entryPrice = quote.Bid
			}
		}
	}
	if entryPrice <= 0 {
		entryPrice = math.Max(quote.Ask, quote.Bid)
	}

	orderValueCostPerLot := commissionRate * entryPrice * contractSize
	totalCostPerLot := commissionFixedPerLot + orderValueCostPerLot

	baseAttrs = append(baseAttrs,
		attribute.Float64("entry_price", entryPrice),
		attribute.Float64("order_value_cost_per_lot", orderValueCostPerLot),
	)

	lotTargetRaw, err := domain.CalculateLotByRiskWithCosts(distancePoints, tickValue, policy.Amount, totalCostPerLot)
	if err != nil {
		result.Reason = "calculation_failed"
		e.recordError(ctx, err)
		e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, result.Decision, result.Reason, append(baseAttrs, attribute.Float64("distance_points", distancePoints))...)
		return result, err
	}

	lotTarget := lotTargetRaw
	minOverrideApplied := false
	maxOverrideApplied := false
	if policy.MinLotOverride != nil && *policy.MinLotOverride > 0 && lotTarget < *policy.MinLotOverride {
		if e.telemetry != nil {
			e.telemetry.Info(ctx, "Fixed risk min lot override applied",
				append(baseAttrs,
					attribute.Float64("lot_target_original", lotTarget),
					attribute.Float64("min_lot_override", *policy.MinLotOverride),
				)...)
		}
		lotTarget = *policy.MinLotOverride
		minOverrideApplied = true
	}

	if policy.MaxLotOverride != nil && *policy.MaxLotOverride > 0 {
		newTarget := math.Min(lotTarget, *policy.MaxLotOverride)
		if newTarget != lotTarget {
			if e.telemetry != nil {
				e.telemetry.Info(ctx, "Fixed risk max lot override applied",
					append(baseAttrs,
						attribute.Float64("lot_target_original", lotTarget),
						attribute.Float64("max_lot_override", *policy.MaxLotOverride),
					)...)
			}
			lotTarget = newTarget
			maxOverrideApplied = true
		}
	}

	lotTargetAdjusted := lotTarget
	lotFinal := lotTargetAdjusted
	guardDecision := volumeguard.DecisionPassThrough
	if e.guard != nil {
		lotFinal, guardDecision, err = e.guard.Execute(ctx, accountID, canonicalSymbol, strategyID, lotTargetAdjusted)
		if err != nil {
			commissionTotalWithError := lotFinal * totalCostPerLot
			result.Reason = "volume_guard_error"
			e.recordError(ctx, err)
			e.recordCalculation(ctx, DecisionReject, accountID, strategyID, canonicalSymbol, result.Reason)
			e.logOutcome(ctx, DecisionReject, result.Reason, append(baseAttrs, attribute.Float64("lot_target", lotTarget))...)
			return Result{
				Decision:              DecisionReject,
				Reason:                result.Reason,
				GuardDecision:         guardDecision,
				CommissionFixedPerLot: commissionFixedPerLot,
				CommissionPerLot:      totalCostPerLot,
				CommissionRate:        commissionRate,
				CommissionTotal:       commissionTotalWithError,
			}, err
		}
		if guardDecision == volumeguard.DecisionReject {
			commissionTotalOnReject := lotFinal * totalCostPerLot
			result.Reason = "volume_guard_reject"
			e.recordCalculation(ctx, DecisionReject, accountID, strategyID, canonicalSymbol, result.Reason)
			e.logOutcome(ctx, DecisionReject, result.Reason, append(baseAttrs, attribute.Float64("lot_target", lotTarget))...)
			return Result{
				Decision:              DecisionReject,
				Reason:                result.Reason,
				GuardDecision:         guardDecision,
				CommissionFixedPerLot: commissionFixedPerLot,
				CommissionPerLot:      totalCostPerLot,
				CommissionRate:        commissionRate,
				CommissionTotal:       commissionTotalOnReject,
			}, nil
		}
	}

	riskPerLot := distancePoints*tickValue + totalCostPerLot
	commissionTotal := lotFinal * totalCostPerLot
	expectedLoss := lotFinal * riskPerLot

	allowedDeviation := e.config.MaxRiskDrift
	if guardDecision == volumeguard.DecisionClamp && lotTargetAdjusted > 0 {
		allowedDeviation += math.Abs(lotTargetAdjusted-lotFinal) / lotTargetAdjusted
	}
	if minOverrideApplied && lotTargetRaw > 0 {
		allowedDeviation += math.Abs(lotTargetAdjusted-lotTargetRaw) / lotTargetRaw
	}
	if maxOverrideApplied && lotTargetRaw > 0 {
		allowedDeviation += math.Abs(lotTargetRaw-lotTargetAdjusted) / lotTargetRaw
	}

	relativeDeviation := math.Abs(expectedLoss-policy.Amount) / policy.Amount
	if relativeDeviation > allowedDeviation {
		result.Reason = "risk_drift_exceeded"
		err := fmt.Errorf("expected loss %.4f outside allowed tolerance (allowed %.4f, got %.4f)", expectedLoss, allowedDeviation, relativeDeviation)
		e.recordError(ctx, err)
		e.recordCalculation(ctx, DecisionReject, accountID, strategyID, canonicalSymbol, result.Reason)
		e.logOutcome(ctx, DecisionReject, result.Reason, append(baseAttrs,
			attribute.Float64("expected_loss", expectedLoss),
			semconv.Echo.RiskCommissionPerLot.Float64(totalCostPerLot),
			semconv.Echo.RiskCommissionTotal.Float64(commissionTotal),
			semconv.Echo.RiskCommissionRate.Float64(commissionRate),
		)...)
		return Result{
			Decision:              DecisionReject,
			Reason:                result.Reason,
			GuardDecision:         guardDecision,
			CommissionFixedPerLot: commissionFixedPerLot,
			CommissionPerLot:      totalCostPerLot,
			CommissionRate:        commissionRate,
			CommissionTotal:       commissionTotal,
		}, err
	}

	result = Result{
		Lot:                   lotFinal,
		ExpectedLoss:          expectedLoss,
		Decision:              DecisionProceed,
		Reason:                "ok",
		GuardDecision:         guardDecision,
		CommissionFixedPerLot: commissionFixedPerLot,
		CommissionPerLot:      totalCostPerLot,
		CommissionTotal:       commissionTotal,
		CommissionRate:        commissionRate,
	}

	e.recordDistancePoints(ctx, distancePoints, accountID, strategyID, canonicalSymbol)
	e.recordExposure(ctx, expectedLoss, accountID, strategyID, canonicalSymbol)
	e.recordCalculation(ctx, result.Decision, accountID, strategyID, canonicalSymbol, result.Reason)

	if e.telemetry != nil {
		e.telemetry.Info(ctx, "Fixed risk calculation succeed",
			append(baseAttrs,
				attribute.Float64("lot_target", lotTarget),
				attribute.Float64("lot_final", lotFinal),
				attribute.Float64("expected_loss", expectedLoss),
				attribute.Float64("distance_points", distancePoints),
				semconv.Echo.RiskCommissionPerLot.Float64(totalCostPerLot),
				semconv.Echo.RiskCommissionTotal.Float64(commissionTotal),
				semconv.Echo.RiskCommissionRate.Float64(commissionRate),
			)...)
	}

	e.logOutcome(ctx, result.Decision, result.Reason, append(baseAttrs,
		attribute.Float64("lot_final", lotFinal),
		attribute.Float64("expected_loss", expectedLoss),
		attribute.Float64("distance_points", distancePoints),
		semconv.Echo.RiskCommissionPerLot.Float64(totalCostPerLot),
		semconv.Echo.RiskCommissionTotal.Float64(commissionTotal),
		semconv.Echo.RiskCommissionRate.Float64(commissionRate),
	)...)

	return result, nil
}

func (e *FixedRiskEngine) recordError(ctx context.Context, err error) {
	if err == nil || e.telemetry == nil {
		return
	}
	e.telemetry.RecordError(ctx, err)
}

func (e *FixedRiskEngine) recordDistancePoints(ctx context.Context, points float64, accountID, strategyID, canonical string) {
	if e.metrics == nil || points <= 0 {
		return
	}
	e.metrics.RecordFixedRiskDistancePoints(ctx, points,
		semconv.Echo.AccountID.String(accountID),
		semconv.Echo.Strategy.String(strategyID),
		semconv.Echo.Symbol.String(canonical),
	)
}

func (e *FixedRiskEngine) recordExposure(ctx context.Context, amount float64, accountID, strategyID, canonical string) {
	if e.metrics == nil || amount <= 0 {
		return
	}
	e.metrics.RecordFixedRiskExposure(ctx, amount,
		semconv.Echo.AccountID.String(accountID),
		semconv.Echo.Strategy.String(strategyID),
		semconv.Echo.Symbol.String(canonical),
	)
}

func (e *FixedRiskEngine) recordCalculation(ctx context.Context, decision Decision, accountID, strategyID, canonical, reason string) {
	if e.metrics == nil {
		return
	}
	e.metrics.RecordFixedRiskCalculation(ctx, string(decision),
		semconv.Echo.AccountID.String(accountID),
		semconv.Echo.Strategy.String(strategyID),
		semconv.Echo.Symbol.String(canonical),
		attribute.String("risk_reason", reason),
	)
}

func (e *FixedRiskEngine) logInfo(ctx context.Context, message string, attrs ...attribute.KeyValue) {
	if e.telemetry == nil {
		return
	}
	e.telemetry.Info(ctx, message, attrs...)
}

func (e *FixedRiskEngine) logOutcome(ctx context.Context, decision Decision, reason string, attrs ...attribute.KeyValue) {
	if e.telemetry == nil {
		return
	}
	full := append(attrs,
		attribute.String("decision", string(decision)),
		attribute.String("reason", reason),
	)
	e.telemetry.Info(ctx, "Fixed risk evaluation", full...)
}
