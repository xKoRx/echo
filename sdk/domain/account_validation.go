package domain

import (
	"errors"
	"fmt"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// ValidateAccountID valida que un account_id es válido.
//
// En i2 solo valida que no esté vacío.
// Formato futuro i3+ puede incluir validación de formato específico.
func ValidateAccountID(accountID string) error {
	if accountID == "" {
		return errors.New("account_id cannot be empty")
	}
	// Opcional: validar formato (solo dígitos, longitud, etc.)
	// Por ahora solo validamos no-vacío
	return nil
}

// ValidateAccountConnected valida un mensaje AccountConnected.
//
// Validaciones:
//   - account_id no puede estar vacío
//   - connected_at_ms debe ser positivo
func ValidateAccountConnected(msg *pb.AccountConnected) error {
	if msg == nil {
		return NewError(ErrMissingRequiredField, "AccountConnected is nil")
	}

	if err := ValidateAccountID(msg.AccountId); err != nil {
		return fmt.Errorf("invalid account_id: %w", err)
	}

	if msg.ConnectedAtMs <= 0 {
		return NewValidationError("connected_at_ms", msg.ConnectedAtMs, "connected_at_ms must be positive")
	}

	return nil
}

// ValidateAccountDisconnected valida un mensaje AccountDisconnected.
//
// Validaciones:
//   - account_id no puede estar vacío
//   - disconnected_at_ms debe ser positivo
func ValidateAccountDisconnected(msg *pb.AccountDisconnected) error {
	if msg == nil {
		return NewError(ErrMissingRequiredField, "AccountDisconnected is nil")
	}

	if err := ValidateAccountID(msg.AccountId); err != nil {
		return fmt.Errorf("invalid account_id: %w", err)
	}

	if msg.DisconnectedAtMs <= 0 {
		return NewValidationError("disconnected_at_ms", msg.DisconnectedAtMs, "disconnected_at_ms must be positive")
	}

	return nil
}

