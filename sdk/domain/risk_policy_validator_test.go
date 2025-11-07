package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFixedRiskConfig_Success(t *testing.T) {
	raw := []byte(`{"amount":150.5,"currency":"usd","min_lot_override":0.05,"max_lot_override":0.75,"commission_per_lot":12.5,"commission_rate":0.5}`)

	cfg, err := ParseFixedRiskConfig(raw)
	require.NoError(t, err)

	assert.Equal(t, 150.5, cfg.Amount)
	assert.Equal(t, "USD", cfg.Currency)
	if assert.NotNil(t, cfg.MinLotOverride) {
		assert.InDelta(t, 0.05, *cfg.MinLotOverride, 1e-9)
	}
	if assert.NotNil(t, cfg.MaxLotOverride) {
		assert.InDelta(t, 0.75, *cfg.MaxLotOverride, 1e-9)
	}
	if assert.NotNil(t, cfg.CommissionPerLot) {
		assert.InDelta(t, 12.5, *cfg.CommissionPerLot, 1e-9)
	}
	if assert.NotNil(t, cfg.CommissionRate) {
		assert.InDelta(t, 0.5, *cfg.CommissionRate, 1e-9)
	}
}

func TestParseFixedRiskConfig_ValidationErrors(t *testing.T) {
	// Amount missing
	_, err := ParseFixedRiskConfig([]byte(`{"currency":"USD"}`))
	assert.Error(t, err)

	// Invalid currency format
	raw := []byte(`{"amount":100,"currency":"us","max_lot_override":1}`)
	_, err = ParseFixedRiskConfig(raw)
	assert.Error(t, err)

	// Negative override
	data := map[string]any{
		"amount":             100.0,
		"currency":           "USD",
		"min_lot_override":   0.2,
		"max_lot_override":   -0.1,
		"commission_per_lot": 1.0,
	}
	encoded, marshalErr := json.Marshal(data)
	require.NoError(t, marshalErr)
	_, err = ParseFixedRiskConfig(encoded)
	assert.Error(t, err)

	invalidRange := map[string]any{
		"amount":           100.0,
		"currency":         "USD",
		"min_lot_override": 0.6,
		"max_lot_override": 0.4,
	}
	encodedRange, marshalErr := json.Marshal(invalidRange)
	require.NoError(t, marshalErr)
	_, err = ParseFixedRiskConfig(encodedRange)
	assert.Error(t, err)

	// Negative commission
	invalidCommission := map[string]any{
		"amount":             100.0,
		"currency":           "USD",
		"commission_per_lot": -5.0,
	}
	encodedCommission, marshalErr := json.Marshal(invalidCommission)
	require.NoError(t, marshalErr)
	_, err = ParseFixedRiskConfig(encodedCommission)
	assert.Error(t, err)

	invalidRate := map[string]any{
		"amount":          100.0,
		"currency":        "USD",
		"commission_rate": -0.01,
	}
	encodedRate, marshalErr := json.Marshal(invalidRate)
	require.NoError(t, marshalErr)
	_, err = ParseFixedRiskConfig(encodedRate)
	assert.Error(t, err)
}

func TestValidateFixedRiskConfig(t *testing.T) {
	cfg := &FixedRiskConfig{Amount: 50, Currency: "eur"}
	require.NoError(t, NormalizeFixedRiskConfig(cfg))
	require.NoError(t, ValidateFixedRiskConfig(cfg))
	assert.Equal(t, "EUR", cfg.Currency)

	cfg.Amount = 0
	assert.Error(t, ValidateFixedRiskConfig(cfg))
}
