package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var currencyCodeRegex = regexp.MustCompile(`^[A-Z]{3}$`)

// fixedRiskConfigDTO representa el esquema JSON esperado para FIXED_RISK.
type fixedRiskConfigDTO struct {
	Amount           *float64 `json:"amount"`
	Currency         string   `json:"currency"`
	MinLotOverride   *float64 `json:"min_lot_override"`
	MaxLotOverride   *float64 `json:"max_lot_override"`
	CommissionPerLot *float64 `json:"commission_per_lot"`
	CommissionRate   *float64 `json:"commission_rate"`
}

// ParseFixedRiskConfig deserializa y valida la configuración FIXED_RISK proveniente de JSONB.
//
// Ejemplo de uso:
//
//	cfg, err := domain.ParseFixedRiskConfig(rawJSON)
//	if err != nil {
//	    return err
//	}
//	// cfg listo para usar
func ParseFixedRiskConfig(raw json.RawMessage) (*FixedRiskConfig, error) {
	if len(raw) == 0 {
		return nil, NewValidationError("config", nil, "fixed risk config cannot be empty")
	}

	var dto fixedRiskConfigDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return nil, WrapError(ErrPolicyViolation, "failed to unmarshal fixed risk config", err)
	}

	if dto.Amount == nil {
		return nil, NewValidationError("amount", nil, "amount is required")
	}

	cfg := &FixedRiskConfig{
		Amount:           *dto.Amount,
		Currency:         dto.Currency,
		MinLotOverride:   dto.MinLotOverride,
		MaxLotOverride:   dto.MaxLotOverride,
		CommissionPerLot: dto.CommissionPerLot,
		CommissionRate:   dto.CommissionRate,
	}

	if err := NormalizeFixedRiskConfig(cfg); err != nil {
		return nil, err
	}

	if err := ValidateFixedRiskConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// NormalizeFixedRiskConfig aplica transformaciones determinísticas (uppercase de currency).
func NormalizeFixedRiskConfig(cfg *FixedRiskConfig) error {
	if cfg == nil {
		return NewError(ErrMissingRequiredField, "fixed risk config is nil")
	}

	cfg.Currency = strings.ToUpper(strings.TrimSpace(cfg.Currency))

	return nil
}

// ValidateFixedRiskConfig valida los campos de la configuración FIXED_RISK.
func ValidateFixedRiskConfig(cfg *FixedRiskConfig) error {
	if cfg == nil {
		return NewError(ErrMissingRequiredField, "fixed risk config is nil")
	}

	if cfg.Amount <= 0 {
		return NewValidationError("amount", cfg.Amount, "amount must be greater than zero")
	}

	if cfg.Currency == "" {
		return NewValidationError("currency", cfg.Currency, "currency is required")
	}

	if !currencyCodeRegex.MatchString(cfg.Currency) {
		return NewValidationError("currency", cfg.Currency, fmt.Sprintf("currency must match %s", currencyCodeRegex.String()))
	}

	if cfg.MinLotOverride != nil && *cfg.MinLotOverride <= 0 {
		return NewValidationError("min_lot_override", *cfg.MinLotOverride, "min_lot_override must be greater than zero when provided")
	}

	if cfg.MaxLotOverride != nil && *cfg.MaxLotOverride <= 0 {
		return NewValidationError("max_lot_override", *cfg.MaxLotOverride, "max_lot_override must be greater than zero when provided")
	}

	if cfg.MinLotOverride != nil && cfg.MaxLotOverride != nil && *cfg.MinLotOverride > *cfg.MaxLotOverride {
		return NewValidationError("min_lot_override", *cfg.MinLotOverride, "min_lot_override cannot be greater than max_lot_override")
	}

	if cfg.CommissionPerLot != nil && *cfg.CommissionPerLot < 0 {
		return NewValidationError("commission_per_lot", *cfg.CommissionPerLot, "commission_per_lot must be zero or positive when provided")
	}

	if cfg.CommissionRate != nil && *cfg.CommissionRate < 0 {
		return NewValidationError("commission_rate", *cfg.CommissionRate, "commission_rate must be zero or positive when provided")
	}

	return nil
}
