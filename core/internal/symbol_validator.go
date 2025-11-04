// Package internal contiene el validador de símbolos canónicos (i3).
package internal

import (
	"context"
	"fmt"

	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"go.opentelemetry.io/otel/attribute"
)

// UnknownAction define la política cuando un símbolo desconocido no está mapeado.
type UnknownAction string

const (
	UnknownActionWarn   UnknownAction = "warn"   // Log warning y continuar (compatibilidad)
	UnknownActionReject UnknownAction = "reject"  // Rechazar orden (criterio de salida i3)
)

// CanonicalValidator valida símbolos canónicos contra una whitelist (i3).
//
// Responsabilidades:
//   - Validar que un símbolo esté en la whitelist canónica
//   - Normalizar símbolos a su forma canónica
//   - Aplicar política unknown_action (warn/reject)
type CanonicalValidator struct {
	canonicalSymbols []string                // Lista de símbolos canónicos permitidos
	unknownAction    UnknownAction           // Política para símbolos desconocidos
	telemetry        *telemetry.Client
	echoMetrics      *metricbundle.EchoMetrics
}

// NewCanonicalValidator crea un nuevo validador de símbolos canónicos.
func NewCanonicalValidator(canonicalSymbols []string, unknownAction UnknownAction, tel *telemetry.Client, echoMetrics *metricbundle.EchoMetrics) *CanonicalValidator {
	return &CanonicalValidator{
		canonicalSymbols: canonicalSymbols,
		unknownAction:    unknownAction,
		telemetry:        tel,
		echoMetrics:      echoMetrics,
	}
}

// IsValid verifica si un símbolo está en la whitelist canónica.
func (v *CanonicalValidator) IsValid(symbol string) bool {
	return domain.ValidateCanonicalSymbol(symbol, v.canonicalSymbols) == nil
}

// Normalize normaliza un símbolo a su forma canónica.
func (v *CanonicalValidator) Normalize(symbol string) (string, error) {
	return domain.NormalizeCanonical(symbol)
}

// List retorna la lista de símbolos canónicos permitidos.
func (v *CanonicalValidator) List() []string {
	return v.canonicalSymbols
}

// UnknownAction retorna la política para símbolos desconocidos.
func (v *CanonicalValidator) UnknownAction() UnknownAction {
	return v.unknownAction
}

// Validate valida un símbolo y aplica la política unknown_action (i3).
//
// Retorna error si unknown_action=reject y el símbolo no está en whitelist.
// Si unknown_action=warn, registra warning pero retorna nil (compatibilidad).
func (v *CanonicalValidator) Validate(ctx context.Context, symbol string) error {
	if err := domain.ValidateCanonicalSymbol(symbol, v.canonicalSymbols); err != nil {
		normalized, normErr := domain.NormalizeCanonical(symbol)
		if normErr != nil {
			normalized = symbol // Fallback
		}

		attrs := []attribute.KeyValue{
			attribute.String("symbol", symbol),
			attribute.String("normalized", normalized),
			attribute.String("unknown_action", string(v.unknownAction)),
		}

		if v.unknownAction == UnknownActionReject {
			v.echoMetrics.RecordSymbolValidate(ctx, "reject", symbol, attrs...)
			v.telemetry.Error(ctx, "Symbol validation failed (reject)", err, attrs...)
			return fmt.Errorf("symbol %s not in canonical whitelist (normalized: %s): %w", symbol, normalized, err)
		}

		// unknown_action=warn: log y continuar (compatibilidad)
		v.echoMetrics.RecordSymbolValidate(ctx, "warn", symbol, attrs...)
		v.telemetry.Warn(ctx, "Symbol validation failed (warn, continuing)", attrs...)
		return nil
	}

	// Validación exitosa
	v.echoMetrics.RecordSymbolValidate(ctx, "ok", symbol)
	return nil
}

