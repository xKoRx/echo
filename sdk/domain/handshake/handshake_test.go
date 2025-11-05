package handshake

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeHandshakePayload_V2Success(t *testing.T) {
	raw := map[string]interface{}{
		"type":         "handshake",
		"timestamp_ms": float64(1730851200123),
		"payload": map[string]interface{}{
			"account_id":        "123456",
			"client_id":         "slave_123456",
			"broker":            "BrokerX",
			"role":              "slave",
			"protocol_version":  float64(2),
			"client_semver":     "1.5.0",
			"required_features": []interface{}{"spec_report/v1", "spec_report/v1"},
			"optional_features": []interface{}{"quotes/250ms"},
			"capabilities": map[string]interface{}{
				"features": []interface{}{"quotes/250ms", "spec_report/v1"},
				"metrics":  []interface{}{"exposure_v1"},
			},
			"terminal_build": float64(1415),
			"symbols": []interface{}{
				map[string]interface{}{
					"canonical_symbol": "XAUUSD",
					"broker_symbol":    "XAUUSD.m",
					"digits":           float64(2),
					"point":            float64(0.01),
					"tick_size":        float64(0.01),
					"min_lot":          float64(0.1),
					"max_lot":          float64(100),
					"lot_step":         float64(0.1),
					"stop_level":       float64(10),
					"contract_size":    float64(100),
				},
			},
		},
	}

	got, err := NormalizeHandshakePayload(raw)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.False(t, got.Legacy)
	require.Equal(t, ProtocolVersionV2, got.ProtocolVersion)
	require.Equal(t, "1.5.0", got.ClientSemver)
	require.Equal(t, []string{"spec_report/v1"}, got.RequiredFeatures)
	require.Equal(t, []string{"quotes/250ms"}, got.OptionalFeatures)
	require.Equal(t, []string{"quotes/250ms", "spec_report/v1"}, got.Capabilities.Features)
	require.Equal(t, []string{"exposure_v1"}, got.Capabilities.Metrics)
	require.Equal(t, 1415, got.TerminalBuild)
	require.Equal(t, "slave", got.PipeRole)
	require.Equal(t, int64(1730851200123), got.TimestampMs)
	require.Len(t, got.Symbols, 1)
	require.Equal(t, "XAUUSD", got.Symbols[0].CanonicalSymbol)
	require.NotNil(t, got.Symbols[0].ContractSize)
	require.Equal(t, 100.0, *got.Symbols[0].ContractSize)

	meta := got.ToProtoMetadata()
	require.NotNil(t, meta)
	require.Equal(t, int32(ProtocolVersionV2), meta.ProtocolVersion)
	require.Equal(t, "1.5.0", meta.ClientSemver)
	require.Equal(t, []string{"spec_report/v1"}, meta.RequiredFeatures)
	require.NotNil(t, meta.Capabilities)
	require.Equal(t, []string{"quotes/250ms", "spec_report/v1"}, meta.Capabilities.Features)
	require.Equal(t, []string{"exposure_v1"}, meta.Capabilities.Metrics)
}

func TestNormalizeHandshakePayload_Legacy(t *testing.T) {
	raw := map[string]interface{}{
		"type": "handshake",
		"payload": map[string]interface{}{
			"account_id": "123",
			"client_id":  "slave_legacy",
		},
	}

	got, err := NormalizeHandshakePayload(raw)
	require.NoError(t, err)
	require.True(t, got.Legacy)
	require.Equal(t, ProtocolVersionV1, got.ProtocolVersion)
	require.Empty(t, got.ClientSemver)
	require.Nil(t, got.RequiredFeatures)
	require.Nil(t, got.OptionalFeatures)
	require.Nil(t, got.Capabilities.Features)
	require.Nil(t, got.Capabilities.Metrics)

	meta := got.ToProtoMetadata()
	require.NotNil(t, meta)
	require.Equal(t, int32(ProtocolVersionV1), meta.ProtocolVersion)
	require.Empty(t, meta.ClientSemver)
	require.Nil(t, meta.RequiredFeatures)
	require.Nil(t, meta.OptionalFeatures)
	require.Nil(t, meta.Capabilities)
}

func TestNormalizeHandshakePayload_InvalidSemver(t *testing.T) {
	raw := map[string]interface{}{
		"type": "handshake",
		"payload": map[string]interface{}{
			"protocol_version": float64(2),
			"client_semver":    "1.0",
		},
	}

	_, err := NormalizeHandshakePayload(raw)
	require.ErrorIs(t, err, errInvalidSemver)
}

func TestNormalizeHandshakePayload_InvalidProtocolVersion(t *testing.T) {
	raw := map[string]interface{}{
		"type": "handshake",
		"payload": map[string]interface{}{
			"protocol_version": "abc",
		},
	}

	_, err := NormalizeHandshakePayload(raw)
	require.ErrorIs(t, err, errProtocolVersion)
}

func TestNormalizeHandshakePayload_InvalidCapabilities(t *testing.T) {
	raw := map[string]interface{}{
		"type": "handshake",
		"payload": map[string]interface{}{
			"protocol_version": float64(2),
			"client_semver":    "1.2.3",
			"capabilities":     "not-an-object",
		},
	}

	_, err := NormalizeHandshakePayload(raw)
	require.ErrorIs(t, err, errInvalidCapabilities)
}

func TestSupportsHelpers(t *testing.T) {
	features := []string{"spec_report/v1", "quotes/250ms"}
	require.True(t, Supports(features, "spec_report/v1"))
	require.False(t, Supports(features, "exposure_v1"))

	cap := CapabilitySet{Features: features}
	require.True(t, cap.Supports("quotes/250ms"))
	require.False(t, cap.Supports("risk_sync"))
}

func TestVersionRange(t *testing.T) {
	vr, err := NewVersionRange(1, 3, []int{2})
	require.NoError(t, err)
	require.True(t, vr.Contains(1))
	require.True(t, vr.Contains(3))
	require.False(t, vr.Contains(4))
	require.True(t, vr.IsBlocked(2))
	require.False(t, vr.IsBlocked(1))
}
