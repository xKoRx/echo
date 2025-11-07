package domain

// CalculateLotByRisk calcula el tamaño de lote requerido para alcanzar un monto de riesgo deseado.
//
// Fórmula:
//
//	lot = riskAmount / (distancePoints * tickValue)
//
// Donde:
//   - distancePoints: distancia entre precio de entrada y stop loss en puntos (>= 0)
//   - tickValue: valor monetario de un punto por lote (>= 0)
//   - riskAmount: monto monetario que se desea arriesgar
func CalculateLotByRisk(distancePoints, tickValue, riskAmount float64) (float64, error) {
	return CalculateLotByRiskWithCosts(distancePoints, tickValue, riskAmount, 0)
}

// CalculateLotByRiskWithCosts amplía el cálculo considerando costos fijos por lote (comisiones).
//
// Fórmula:
//
//	lot = riskAmount / ((distancePoints * tickValue) + extraCostPerLot)
//
// Donde extraCostPerLot representa el costo monetario por lote (por ejemplo, comisión round-trip).
func CalculateLotByRiskWithCosts(distancePoints, tickValue, riskAmount, extraCostPerLot float64) (float64, error) {
	if distancePoints <= 0 {
		return 0, NewValidationError("distance_points", distancePoints, "distance_points must be greater than zero")
	}

	if tickValue <= 0 {
		return 0, NewValidationError("tick_value", tickValue, "tick_value must be greater than zero")
	}

	if riskAmount <= 0 {
		return 0, NewValidationError("risk_amount", riskAmount, "risk_amount must be greater than zero")
	}

	if extraCostPerLot < 0 {
		return 0, NewValidationError("extra_cost_per_lot", extraCostPerLot, "extra_cost_per_lot must be zero or positive")
	}

	riskPerLot := distancePoints*tickValue + extraCostPerLot
	if riskPerLot <= 0 {
		return 0, NewError(ErrInvalidSpec, "risk per lot must be positive")
	}

	lot := riskAmount / riskPerLot
	if lot <= 0 {
		return 0, NewValidationError("lot", lot, "calculated lot must be greater than zero")
	}

	return lot, nil
}
