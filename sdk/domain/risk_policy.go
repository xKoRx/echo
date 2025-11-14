package domain

import (
	"context"
	"time"
)

// RiskPolicyType identifica el tipo de política de riesgo.
type RiskPolicyType string

const (
	// RiskPolicyTypeFixedLot representa una política de lote fijo (iteración 4).
	RiskPolicyTypeFixedLot RiskPolicyType = "FIXED_LOT"

	// RiskPolicyTypeFixedRisk representa una política de riesgo fijo (iteración 6).
	RiskPolicyTypeFixedRisk RiskPolicyType = "FIXED_RISK"
)

// FixedLotConfig almacena la configuración de una política FIXED_LOT.
type FixedLotConfig struct {
	LotSize float64
}

// FixedRiskConfig almacena la configuración de una política FIXED_RISK.
type FixedRiskConfig struct {
	Amount           float64
	Currency         string
	MinLotOverride   *float64
	MaxLotOverride   *float64
	CommissionPerLot *float64
	CommissionRate   *float64
}

// RiskPolicy representa una política de riesgo por cuenta × estrategia.
type RiskPolicy struct {
	AccountID  string
	StrategyID string
	Type       RiskPolicyType
	FixedLot   *FixedLotConfig
	FixedRisk  *FixedRiskConfig
	Version    int64
	UpdatedAt  time.Time
	ValidUntil *time.Time
}

// RiskPolicyService encapsula la lógica de caché y lectura de políticas.
type RiskPolicyService interface {
	Get(ctx context.Context, accountID, strategyID string) (*RiskPolicy, error)
	GetAdjustableStops(ctx context.Context, accountID, symbol string) (*AdjustableStops, error)
	Invalidate(accountID, strategyID string)
}
