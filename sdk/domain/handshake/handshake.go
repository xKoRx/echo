package handshake

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/utils"
)

const (
	// ProtocolVersionV1 representa la versión legacy del handshake.
	ProtocolVersionV1 = 1
	// ProtocolVersionV2 representa la versión introducida en i5.
	ProtocolVersionV2 = 2
	// ProtocolVersionV3 representa la versión introducida en i17 (End-to-End Lossless Retry).
	ProtocolVersionV3 = 3
)

var (
	semverRegexp           = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	errHandshakePayload    = errors.New("handshake payload missing or invalid")
	errProtocolVersion     = errors.New("handshake protocol_version must be a positive integer")
	errInvalidSemver       = errors.New("handshake client_semver must match MAJOR.MINOR.PATCH")
	errInvalidCapabilities = errors.New("handshake capabilities must be an object")
)

// Handshake encapsula la información normalizada del handshake EA → Agent.
//
// Incluye metadata obligatoria para i5 (protocol_version, client_semver,
// capabilities) y los campos legacy de i1-i4 para compatibilidad.
type Handshake struct {
	Type             string
	TimestampMs      int64
	AccountID        string
	ClientID         string
	Broker           string
	PipeRole         string
	ProtocolVersion  int
	ClientSemver     string
	RequiredFeatures []string
	OptionalFeatures []string
	Capabilities     CapabilitySet
	Symbols          []*pb.SymbolMapping
	TerminalBuild    int
	Legacy           bool
	Raw              map[string]interface{}
}

// CapabilitySet lista las capacidades negociadas en el handshake.
type CapabilitySet struct {
	Features []string
	Metrics  []string
}

// Supports indica si un slice de features incluye la feature solicitada.
func Supports(features []string, feature string) bool {
	for _, f := range features {
		if f == feature {
			return true
		}
	}
	return false
}

// Supports indica si el set de capacidades incluye la feature solicitada.
func (c CapabilitySet) Supports(feature string) bool {
	return Supports(c.Features, feature)
}

// VersionRange define un rango de versiones válido para el handshake.
type VersionRange struct {
	Min     int
	Max     int
	Blocked map[int]struct{}
}

// NewVersionRange construye un rango de versiones validado.
func NewVersionRange(min, max int, blocked []int) (*VersionRange, error) {
	vr := &VersionRange{
		Min:     min,
		Max:     max,
		Blocked: make(map[int]struct{}, len(blocked)),
	}
	for _, v := range blocked {
		vr.Blocked[v] = struct{}{}
	}
	if err := vr.Validate(); err != nil {
		return nil, err
	}
	return vr, nil
}

// Validate asegura que el rango cumple min ≤ max y descarta inconsistencias.
func (vr *VersionRange) Validate() error {
	if vr == nil {
		return errors.New("version range is nil")
	}
	if vr.Min <= 0 || vr.Max <= 0 {
		return fmt.Errorf("version range must be positive (min=%d max=%d)", vr.Min, vr.Max)
	}
	if vr.Min > vr.Max {
		return fmt.Errorf("version range min (%d) greater than max (%d)", vr.Min, vr.Max)
	}
	for blocked := range vr.Blocked {
		if blocked < vr.Min || blocked > vr.Max {
			return fmt.Errorf("blocked version %d outside of range [%d,%d]", blocked, vr.Min, vr.Max)
		}
	}
	return nil
}

// Contains indica si el rango incluye la versión dada.
func (vr *VersionRange) Contains(version int) bool {
	if vr == nil {
		return false
	}
	return version >= vr.Min && version <= vr.Max
}

// IsBlocked indica si la versión está bloqueada explícitamente.
func (vr *VersionRange) IsBlocked(version int) bool {
	if vr == nil {
		return false
	}
	_, ok := vr.Blocked[version]
	return ok
}

// RegistrationStatus representa el estado global o por símbolo del registro.
type RegistrationStatus pb.SymbolRegistrationStatus

const (
	RegistrationStatusUnspecified RegistrationStatus = RegistrationStatus(pb.SymbolRegistrationStatus_SYMBOL_REGISTRATION_STATUS_UNSPECIFIED)
	RegistrationStatusAccepted    RegistrationStatus = RegistrationStatus(pb.SymbolRegistrationStatus_SYMBOL_REGISTRATION_STATUS_ACCEPTED)
	RegistrationStatusWarning     RegistrationStatus = RegistrationStatus(pb.SymbolRegistrationStatus_SYMBOL_REGISTRATION_STATUS_WARNING)
	RegistrationStatusRejected    RegistrationStatus = RegistrationStatus(pb.SymbolRegistrationStatus_SYMBOL_REGISTRATION_STATUS_REJECTED)
)

// ToProto convierte el estado a enum proto.
func (s RegistrationStatus) ToProto() pb.SymbolRegistrationStatus {
	return pb.SymbolRegistrationStatus(s)
}

// Issue representa una advertencia/error en el registro.
type Issue struct {
	Code     IssueCode
	Message  string
	Metadata map[string]string
}

// ToProto convierte la issue a mensaje proto.
func (i Issue) ToProto() *pb.SymbolRegistrationIssue {
	protoIssue := &pb.SymbolRegistrationIssue{
		Code:    i.Code.Proto(),
		Message: i.Message,
	}
	if len(i.Metadata) > 0 {
		protoIssue.Metadata = make(map[string]string, len(i.Metadata))
		for k, v := range i.Metadata {
			protoIssue.Metadata[k] = v
		}
	}
	return protoIssue
}

// Entry representa el resultado por símbolo.
type Entry struct {
	CanonicalSymbol string
	BrokerSymbol    string
	Status          RegistrationStatus
	Warnings        []Issue
	Errors          []Issue
	SpecAgeMs       int64
	EvaluatedAtMs   int64
}

// ToProto convierte la entrada a proto.
func (e Entry) ToProto() *pb.SymbolRegistrationEntry {
	protoEntry := &pb.SymbolRegistrationEntry{
		CanonicalSymbol: e.CanonicalSymbol,
		BrokerSymbol:    e.BrokerSymbol,
		Status:          e.Status.ToProto(),
		SpecAgeMs:       e.SpecAgeMs,
		EvaluatedAtMs:   e.EvaluatedAtMs,
	}
	for _, warn := range e.Warnings {
		protoEntry.Warnings = append(protoEntry.Warnings, warn.ToProto())
	}
	for _, err := range e.Errors {
		protoEntry.Errors = append(protoEntry.Errors, err.ToProto())
	}
	return protoEntry
}

// Evaluation representa el resultado global de registro de símbolos.
type Evaluation struct {
	EvaluationID     string
	AccountID        string
	PipeRole         string
	ProtocolVersion  int
	AgentID          string
	CoreVersion      string
	Status           RegistrationStatus
	Errors           []Issue
	Warnings         []Issue
	Entries          []Entry
	EvaluatedAtMs    int64
	ClientSemver     string
	RequiredFeatures []string
	OptionalFeatures []string
	Capabilities     CapabilitySet
}

// NewEvaluation crea un Evaluation con defaults.
func NewEvaluation(accountID, pipeRole string) *Evaluation {
	return &Evaluation{
		AccountID:     accountID,
		PipeRole:      pipeRole,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Status:        RegistrationStatusUnspecified,
	}
}

// ToProtoResult convierte la evaluación en SymbolRegistrationResult.
func (e *Evaluation) ToProtoResult() *pb.SymbolRegistrationResult {
	if e == nil {
		return nil
	}
	result := &pb.SymbolRegistrationResult{
		AccountId:       e.AccountID,
		PipeRole:        e.PipeRole,
		ProtocolVersion: int32(e.ProtocolVersion),
		AgentId:         e.AgentID,
		CoreVersion:     e.CoreVersion,
		Status:          e.Status.ToProto(),
		EvaluatedAtMs:   e.EvaluatedAtMs,
		EvaluationId:    e.EvaluationID,
		ClientSemver:    e.ClientSemver,
	}
	for _, issue := range e.Errors {
		result.Errors = append(result.Errors, issue.ToProto())
	}
	for _, warn := range e.Warnings {
		result.Warnings = append(result.Warnings, warn.ToProto())
	}
	for _, entry := range e.Entries {
		result.Symbols = append(result.Symbols, entry.ToProto())
	}
	return result
}

// IssueCode mapea los códigos de problema de registro de símbolos a nivel dominio.
type IssueCode pb.SymbolRegistrationIssueCode

// Issue codes alineados con proto SymbolRegistrationIssueCode.
const (
	IssueCodeUnspecified                IssueCode = IssueCode(pb.SymbolRegistrationIssueCode_SYMBOL_REGISTRATION_ISSUE_CODE_UNSPECIFIED)
	IssueCodeCanonicalNotAllowed        IssueCode = IssueCode(pb.SymbolRegistrationIssueCode_SYMBOL_REGISTRATION_ISSUE_CODE_CANONICAL_NOT_ALLOWED)
	IssueCodeSpecStale                  IssueCode = IssueCode(pb.SymbolRegistrationIssueCode_SYMBOL_REGISTRATION_ISSUE_CODE_SPEC_STALE)
	IssueCodeRiskPolicyMissing          IssueCode = IssueCode(pb.SymbolRegistrationIssueCode_SYMBOL_REGISTRATION_ISSUE_CODE_RISK_POLICY_MISSING)
	IssueCodeProtocolVersionUnsupported IssueCode = IssueCode(pb.SymbolRegistrationIssueCode_SYMBOL_REGISTRATION_ISSUE_CODE_PROTOCOL_VERSION_UNSUPPORTED)
	IssueCodeFeatureMissing             IssueCode = IssueCode(pb.SymbolRegistrationIssueCode_SYMBOL_REGISTRATION_ISSUE_CODE_FEATURE_MISSING)
)

// Proto convierte el IssueCode de dominio a enum proto.
func (c IssueCode) Proto() pb.SymbolRegistrationIssueCode {
	return pb.SymbolRegistrationIssueCode(c)
}

// String retorna la representación textual del IssueCode.
func (c IssueCode) String() string {
	return pb.SymbolRegistrationIssueCode(c).String()
}

// NormalizeHandshakePayload valida y normaliza el handshake proveniente del EA.
//
// Retorna un Handshake listo para ser consumido por Agent/Core. En caso de
// handshake legacy (sin protocol_version) se marca Legacy=true y se asigna
// ProtocolVersionV1 para compatibilidad.
func NormalizeHandshakePayload(raw map[string]interface{}) (*Handshake, error) {
	if raw == nil {
		return nil, fmt.Errorf("handshake payload is nil: %w", errHandshakePayload)
	}

	payload, ok := raw["payload"].(map[string]interface{})
	if !ok {
		return nil, errHandshakePayload
	}

	protocolVersion, legacy, err := parseProtocolVersion(payload)
	if err != nil {
		return nil, err
	}

	clientSemver, err := parseClientSemver(payload, legacy)
	if err != nil {
		return nil, err
	}

	requiredFeatures := normalizeStringSlice(payload["required_features"])
	optionalFeatures := normalizeStringSlice(payload["optional_features"])
	capabilities, err := parseCapabilities(payload["capabilities"])
	if err != nil {
		return nil, err
	}

	symbols := parseHandshakeSymbols(payload["symbols"])

	terminalBuild := parseOptionalInt(payload["terminal_build"])

	handshake := &Handshake{
		Type:             strings.TrimSpace(utils.ExtractString(raw, "type")),
		TimestampMs:      utils.ExtractInt64(raw, "timestamp_ms"),
		AccountID:        strings.TrimSpace(utils.ExtractString(payload, "account_id")),
		ClientID:         strings.TrimSpace(utils.ExtractString(payload, "client_id")),
		Broker:           strings.TrimSpace(utils.ExtractString(payload, "broker")),
		PipeRole:         strings.TrimSpace(utils.ExtractString(payload, "role")),
		ProtocolVersion:  protocolVersion,
		ClientSemver:     clientSemver,
		RequiredFeatures: requiredFeatures,
		OptionalFeatures: optionalFeatures,
		Capabilities:     capabilities,
		Symbols:          symbols,
		TerminalBuild:    terminalBuild,
		Legacy:           legacy,
		Raw:              raw,
	}

	if handshake.PipeRole == "" {
		handshake.PipeRole = strings.TrimSpace(utils.ExtractString(payload, "pipe_role"))
	}

	return handshake, nil
}

// ToProtoMetadata convierte la metadata negociada a pb.HandshakeMetadata.
func (h *Handshake) ToProtoMetadata() *pb.HandshakeMetadata {
	if h == nil {
		return nil
	}
	meta := &pb.HandshakeMetadata{
		ProtocolVersion:  int32(h.ProtocolVersion),
		ClientSemver:     h.ClientSemver,
		RequiredFeatures: append([]string(nil), h.RequiredFeatures...),
		OptionalFeatures: append([]string(nil), h.OptionalFeatures...),
	}
	if len(h.Capabilities.Features) > 0 || len(h.Capabilities.Metrics) > 0 {
		meta.Capabilities = &pb.HandshakeCapabilities{
			Features: append([]string(nil), h.Capabilities.Features...),
			Metrics:  append([]string(nil), h.Capabilities.Metrics...),
		}
	}
	return meta
}

func parseProtocolVersion(payload map[string]interface{}) (int, bool, error) {
	value, ok := payload["protocol_version"]
	if !ok {
		return ProtocolVersionV1, true, nil
	}

	version, err := asPositiveInt(value)
	if err != nil {
		return 0, false, errProtocolVersion
	}
	if version <= 0 {
		return 0, false, errProtocolVersion
	}

	legacy := version < ProtocolVersionV2
	return version, legacy, nil
}

func parseClientSemver(payload map[string]interface{}, legacy bool) (string, error) {
	raw := strings.TrimSpace(utils.ExtractString(payload, "client_semver"))
	if raw == "" {
		if legacy {
			return "", nil
		}
		return "", errInvalidSemver
	}
	if !semverRegexp.MatchString(raw) {
		return "", errInvalidSemver
	}
	return raw, nil
}

func parseCapabilities(value interface{}) (CapabilitySet, error) {
	if value == nil {
		return CapabilitySet{}, nil
	}
	capMap, ok := value.(map[string]interface{})
	if !ok {
		return CapabilitySet{}, errInvalidCapabilities
	}
	features := normalizeStringSlice(capMap["features"])
	metrics := normalizeStringSlice(capMap["metrics"])
	return CapabilitySet{Features: features, Metrics: metrics}, nil
}

func parseHandshakeSymbols(value interface{}) []*pb.SymbolMapping {
	result := []*pb.SymbolMapping{}
	entries, ok := value.([]interface{})
	if !ok {
		return result
	}
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		canonical := strings.TrimSpace(utils.ExtractString(entryMap, "canonical_symbol"))
		if canonical == "" {
			continue
		}
		mapping := &pb.SymbolMapping{
			CanonicalSymbol: canonical,
			BrokerSymbol:    strings.TrimSpace(utils.ExtractString(entryMap, "broker_symbol")),
			Digits:          int32(utils.ExtractInt64(entryMap, "digits")),
			Point:           utils.ExtractFloat64(entryMap, "point"),
			TickSize:        utils.ExtractFloat64(entryMap, "tick_size"),
			MinLot:          utils.ExtractFloat64(entryMap, "min_lot"),
			MaxLot:          utils.ExtractFloat64(entryMap, "max_lot"),
			LotStep:         utils.ExtractFloat64(entryMap, "lot_step"),
			StopLevel:       int32(utils.ExtractInt64(entryMap, "stop_level")),
		}
		if contract, ok := entryMap["contract_size"].(float64); ok {
			mapping.ContractSize = &contract
		}
		result = append(result, mapping)
	}
	return result
}

func normalizeStringSlice(value interface{}) []string {
	if value == nil {
		return nil
	}
	var items []string
	switch v := value.(type) {
	case []string:
		items = append(items, v...)
	case []interface{}:
		for _, item := range v {
			switch s := item.(type) {
			case string:
				items = append(items, s)
			case fmt.Stringer:
				items = append(items, s.String())
			}
		}
	case string:
		if v != "" {
			items = append(items, v)
		}
	}

	if len(items) == 0 {
		return nil
	}

	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}

	set := make(map[string]struct{}, len(items))
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, exists := set[item]; exists {
			continue
		}
		set[item] = struct{}{}
		filtered = append(filtered, item)
	}

	sort.Strings(filtered)
	return filtered
}

func parseOptionalInt(value interface{}) int {
	if value == nil {
		return 0
	}
	parsed, err := asPositiveInt(value)
	if err != nil {
		return 0
	}
	return parsed
}

func asPositiveInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float32:
		return numericFloatToInt(float64(v))
	case float64:
		return numericFloatToInt(v)
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("empty numeric string")
		}
		i, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", value)
	}
}

func numericFloatToInt(f float64) (int, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid float value")
	}
	intPart, frac := math.Modf(f)
	if math.Abs(frac) > 1e-9 {
		return 0, fmt.Errorf("float value is not an integer")
	}
	return int(intPart), nil
}
