package domain

import (
	"math"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

const floatTolerance = 1e-9

// ClampLotSize valida un tamaño de lote contra una VolumeSpec y retorna el valor clamped.
//
// Si el lot size ya es válido se retorna sin modificar y error nil. Cuando el valor se clampa
// por mínimos/máximos o por step inválido, retorna el valor ajustado junto a un ValidationError
// que describe la causa del clamp. Si la especificación es inválida se retorna un error fatal
// (ErrSpecMissing).
func ClampLotSize(spec *pb.VolumeSpec, lot float64) (float64, error) {
	if spec == nil {
		return 0, NewError(ErrSpecMissing, "volume spec is nil")
	}

	if spec.MinVolume <= 0 {
		return 0, NewValidationError("min_volume", spec.MinVolume, "min_volume must be > 0")
	}
	if spec.MaxVolume <= 0 {
		return 0, NewValidationError("max_volume", spec.MaxVolume, "max_volume must be > 0")
	}
	if spec.VolumeStep <= 0 {
		return 0, NewValidationError("volume_step", spec.VolumeStep, "volume_step must be > 0")
	}

	min := spec.MinVolume
	max := spec.MaxVolume
	step := spec.VolumeStep

	if min > max {
		return 0, NewValidationError("min_volume", min, "min_volume cannot exceed max_volume")
	}

	original := lot
	if lot <= 0 {
		lot = min
	}

	// Clamp por límites
	var validationErr error
	if lot < min {
		validationErr = NewValidationError("lot_size", original, "lot size below minimum")
		lot = min
	}
	if lot > max {
		validationErr = NewValidationError("lot_size", original, "lot size above maximum")
		lot = max
	}

	// Ajustar al múltiplo más cercano del step
	normalized := normalizeToStep(lot, step)
	if normalized < min-floatTolerance {
		normalized = min
	}
	if normalized > max+floatTolerance {
		normalized = max
	}

	if !almostEqual(normalized, lot) && validationErr == nil {
		validationErr = NewValidationError("lot_size", original, "lot size not aligned to volume_step")
	}

	// Si el valor original era válido no retornamos error
	if validationErr == nil && !almostEqual(original, normalized) {
		validationErr = NewValidationError("lot_size", original, "lot size clamped to spec")
	}

	return normalized, validationErr
}

func normalizeToStep(value, step float64) float64 {
	if step <= 0 {
		return value
	}
	quotient := math.Round(value / step)
	return quotient * step
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= floatTolerance
}
