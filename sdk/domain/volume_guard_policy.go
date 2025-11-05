package domain

import "time"

// VolumeGuardMissingSpecPolicy define cómo actuar ante ausencia de especificaciones.
type VolumeGuardMissingSpecPolicy string

const (
	// VolumeGuardMissingSpecReject fuerza rechazo cuando falta la especificación.
	VolumeGuardMissingSpecReject VolumeGuardMissingSpecPolicy = "reject"
)

// VolumeGuardPolicy configura el comportamiento del guardián de volumen en Core.
type VolumeGuardPolicy struct {
	OnMissingSpec VolumeGuardMissingSpecPolicy
	MaxSpecAge    time.Duration
	AlertThreshold time.Duration
	DefaultLot    float64
}

// Validate asegura que la política es consistente.
func (p *VolumeGuardPolicy) Validate() error {
	if p == nil {
		return NewError(ErrPolicyViolation, "volume guard policy is nil")
	}
	if p.OnMissingSpec != VolumeGuardMissingSpecReject {
		return NewValidationError("on_missing_spec", p.OnMissingSpec, "unsupported missing spec policy")
	}
	if p.MaxSpecAge <= 0 {
		return NewValidationError("max_spec_age", p.MaxSpecAge, "max_spec_age must be positive")
	}
	if p.AlertThreshold < 0 {
		return NewValidationError("alert_threshold", p.AlertThreshold, "alert_threshold cannot be negative")
	}
	if p.DefaultLot <= 0 {
		return NewValidationError("default_lot", p.DefaultLot, "default_lot must be positive")
	}
	return nil
}

