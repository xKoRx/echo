package domain

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateLotByRisk_Success(t *testing.T) {
	lot, err := CalculateLotByRisk(5000, 1, 100)
	require.NoError(t, err)
	assert.InDelta(t, 0.02, lot, 1e-9)
}

func TestCalculateLotByRisk_InvalidInputs(t *testing.T) {
	tests := []struct {
		name           string
		distancePoints float64
		tickValue      float64
		riskAmount     float64
	}{
		{"zero distance", 0, 1, 100},
		{"zero tick", 5000, 0, 100},
		{"zero risk", 5000, 1, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CalculateLotByRisk(tc.distancePoints, tc.tickValue, tc.riskAmount)
			assert.Error(t, err)
		})
	}
}

func TestCalculateLotByRisk_RiskPerLotPositive(t *testing.T) {
	lot, err := CalculateLotByRisk(2000, 1.5, 1500)
	require.NoError(t, err)
	assert.True(t, lot > 0 && !math.IsInf(lot, 0) && !math.IsNaN(lot))

	lot, err = CalculateLotByRisk(1e6, 0.01, 5e3)
	require.NoError(t, err)
	assert.True(t, lot > 0)
}

func TestCalculateLotByRiskWithCosts(t *testing.T) {
	riskAmount := 115.65
	extraCostPerLot := 15.65
	distancePoints := 5000.0
	tickValue := 1.0

	lot, err := CalculateLotByRiskWithCosts(distancePoints, tickValue, riskAmount, extraCostPerLot)
	require.NoError(t, err)
	expectedLot := riskAmount / (distancePoints*tickValue + extraCostPerLot)
	assert.InDelta(t, expectedLot, lot, 1e-9)

	_, err = CalculateLotByRiskWithCosts(5000, 1, 100, -1)
	assert.Error(t, err)
}
