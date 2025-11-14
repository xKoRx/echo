package capabilities

import (
	"context"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// StopLevelDecision describe la decisión calculada por el guardián de StopLevel.
type StopLevelDecision string

const (
	// StopLevelDecisionAcceptWithOffsets indica que la orden puede enviarse aplicando offsets.
	StopLevelDecisionAcceptWithOffsets StopLevelDecision = "ACCEPT_WITH_OFFSETS"

	// StopLevelDecisionAcceptWithPostModify indica que se debe encolar una modificación post fill.
	StopLevelDecisionAcceptWithPostModify StopLevelDecision = "ACCEPT_WITH_POST_MODIFY"

	// StopLevelDecisionRejectWithReason indica que la orden debe rechazarse por violar StopLevel.
	StopLevelDecisionRejectWithReason StopLevelDecision = "REJECT_WITH_REASON"
)

// StopLevelGuardRequest encapsula los datos necesarios para evaluar StopLevel.
type StopLevelGuardRequest struct {
	Intent          *pb.TradeIntent
	Policy          *domain.RiskPolicy
	AdjustableStops *domain.AdjustableStops
	Quote           *pb.SymbolQuoteSnapshot
	SymbolInfo      *domain.AccountSymbolInfo
	SymbolSpec      *pb.SymbolSpecification
	AccountID       string
	StrategyID      string
}

// StopLevelGuardResult contiene la evaluación del guardián.
type StopLevelGuardResult struct {
	Decision              StopLevelDecision
	Reason                string
	AdjustableStops       *domain.AdjustableStops
	StopLevelBreach       bool
	EffectiveSLPoints     float64
	EffectiveTPPoints     float64
	AppliedSLOffsetPts    int32
	AppliedTPOffsetPts    int32
	EntryPrice            float64
	TargetStopLossPrice   float64
	TargetTakeProfitPrice float64
	SafeStopLossPrice     float64
	SafeTakeProfitPrice   float64
}

// StopLevelGuard define el contrato para evaluar StopLevel previo al envío de ExecuteOrder.
type StopLevelGuard interface {
	Evaluate(ctx context.Context, req *StopLevelGuardRequest) (*StopLevelGuardResult, error)
}
