package internal

import (
	"fmt"
	"strings"

	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// ProtocolValidator valida metadata de handshake contra la configuración de protocolo.
type ProtocolValidator struct {
	cfg *ProtocolConfig
}

// NewProtocolValidator crea un nuevo validador de protocolo.
func NewProtocolValidator(cfg *ProtocolConfig) *ProtocolValidator {
	return &ProtocolValidator{cfg: cfg}
}

// Validate evalúa la metadata y retorna estado recomendado, errores y warnings.
func (v *ProtocolValidator) Validate(metadata *pb.HandshakeMetadata) (handshake.RegistrationStatus, []handshake.Issue, []handshake.Issue) {
	status := handshake.RegistrationStatusAccepted
	var errors []handshake.Issue
	var warnings []handshake.Issue

	if metadata == nil {
		errors = append(errors, handshake.Issue{
			Code:    handshake.IssueCodeProtocolVersionUnsupported,
			Message: "metadata ausente en AccountSymbolsReport",
		})
		return handshake.RegistrationStatusRejected, errors, warnings
	}

	version := int(metadata.GetProtocolVersion())
	if version <= 0 {
		errors = append(errors, handshake.Issue{
			Code:    handshake.IssueCodeProtocolVersionUnsupported,
			Message: "protocol_version ausente o inválida",
		})
		status = handshake.RegistrationStatusRejected
	} else if v.cfg != nil && v.cfg.VersionRange != nil {
		if !v.cfg.VersionRange.Contains(version) {
			errors = append(errors, handshake.Issue{
				Code:    handshake.IssueCodeProtocolVersionUnsupported,
				Message: fmt.Sprintf("protocol_version %d fuera de rango [%d,%d]", version, v.cfg.VersionRange.Min, v.cfg.VersionRange.Max),
			})
			status = handshake.RegistrationStatusRejected
		} else if v.cfg.VersionRange.IsBlocked(version) {
			errors = append(errors, handshake.Issue{
				Code:    handshake.IssueCodeProtocolVersionUnsupported,
				Message: fmt.Sprintf("protocol_version %d bloqueada por configuración", version),
			})
			status = handshake.RegistrationStatusRejected
		}
	}

	clientSemver := strings.TrimSpace(metadata.GetClientSemver())
	if version >= handshake.ProtocolVersionV2 && clientSemver == "" {
		errors = append(errors, handshake.Issue{
			Code:    handshake.IssueCodeProtocolVersionUnsupported,
			Message: "client_semver requerido para protocolo >= 2",
		})
		status = handshake.RegistrationStatusRejected
	} else if version < handshake.ProtocolVersionV2 && clientSemver == "" {
		warnings = append(warnings, handshake.Issue{
			Code:    handshake.IssueCodeProtocolVersionUnsupported,
			Message: "client_semver ausente (legacy)",
		})
		if status == handshake.RegistrationStatusAccepted {
			status = handshake.RegistrationStatusWarning
		}
	}

	requiredSet := make(map[string]struct{})
	if v.cfg != nil {
		for _, feature := range v.cfg.RequiredFeatures {
			feature = strings.TrimSpace(feature)
			if feature != "" {
				requiredSet[feature] = struct{}{}
			}
		}
	}

	if len(requiredSet) > 0 {
		provided := collectFeatures(metadata)
		missing := make([]string, 0)
		for feature := range requiredSet {
			if _, ok := provided[feature]; !ok {
				missing = append(missing, feature)
			}
		}
		if len(missing) > 0 {
			errors = append(errors, handshake.Issue{
				Code:    handshake.IssueCodeFeatureMissing,
				Message: fmt.Sprintf("features obligatorias ausentes: %s", strings.Join(missing, ",")),
				Metadata: map[string]string{
					"missing": strings.Join(missing, ","),
				},
			})
			status = handshake.RegistrationStatusRejected
		}
	}

	return status, errors, warnings
}

func collectFeatures(metadata *pb.HandshakeMetadata) map[string]struct{} {
	result := make(map[string]struct{})
	if metadata == nil {
		return result
	}
	for _, feature := range metadata.GetRequiredFeatures() {
		feature = strings.TrimSpace(feature)
		if feature != "" {
			result[feature] = struct{}{}
		}
	}
	for _, feature := range metadata.GetOptionalFeatures() {
		feature = strings.TrimSpace(feature)
		if feature != "" {
			result[feature] = struct{}{}
		}
	}
	if metadata.Capabilities != nil {
		for _, feature := range metadata.Capabilities.GetFeatures() {
			feature = strings.TrimSpace(feature)
			if feature != "" {
				result[feature] = struct{}{}
			}
		}
	}
	return result
}
