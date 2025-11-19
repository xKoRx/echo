package internal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// HandshakeEvaluator evalúa handshakes, persiste resultados y actualiza métricas.
type HandshakeEvaluator struct {
	protocolValidator  *ProtocolValidator
	canonicalValidator *CanonicalValidator
	symbolSpecService  *SymbolSpecService
	riskPolicyService  domain.RiskPolicyService
	repository         domain.HandshakeEvaluationRepository
	telemetry          *telemetry.Client
	metrics            *metricbundle.EchoMetrics
	protocolCfg        *ProtocolConfig
	specMaxAge         time.Duration
}

// NewHandshakeEvaluator construye un evaluador completo.
func NewHandshakeEvaluator(
	protocolValidator *ProtocolValidator,
	canonicalValidator *CanonicalValidator,
	symbolSpecService *SymbolSpecService,
	riskPolicyService domain.RiskPolicyService,
	repository domain.HandshakeEvaluationRepository,
	telemetryClient *telemetry.Client,
	metrics *metricbundle.EchoMetrics,
	protocolCfg *ProtocolConfig,
	specMaxAge time.Duration,
) *HandshakeEvaluator {
	return &HandshakeEvaluator{
		protocolValidator:  protocolValidator,
		canonicalValidator: canonicalValidator,
		symbolSpecService:  symbolSpecService,
		riskPolicyService:  riskPolicyService,
		repository:         repository,
		telemetry:          telemetryClient,
		metrics:            metrics,
		protocolCfg:        protocolCfg,
		specMaxAge:         specMaxAge,
	}
}

// Evaluate genera una evaluación de registro y la persiste cuando existen cambios.
// Retorna la evaluación efectiva y un booleano indicando si se persistió una nueva
// fila en la base de datos.
func (e *HandshakeEvaluator) Evaluate(
	ctx context.Context,
	accountID string,
	agentID string,
	pipeRole string,
	coreVersion string,
	metadata *pb.HandshakeMetadata,
	mappings []*domain.SymbolMapping,
) (*handshake.Evaluation, bool, error) {
	return e.evaluate(ctx, accountID, agentID, pipeRole, coreVersion, metadata, mappings, nil)
}

// EvaluateWithPrevious permite reutilizar una evaluación previa ya cargada para
// evitar lecturas redundantes desde el repositorio.
func (e *HandshakeEvaluator) EvaluateWithPrevious(
	ctx context.Context,
	accountID string,
	agentID string,
	pipeRole string,
	coreVersion string,
	metadata *pb.HandshakeMetadata,
	mappings []*domain.SymbolMapping,
	previous *handshake.Evaluation,
) (*handshake.Evaluation, bool, error) {
	return e.evaluate(ctx, accountID, agentID, pipeRole, coreVersion, metadata, mappings, previous)
}

func (e *HandshakeEvaluator) evaluate(
	ctx context.Context,
	accountID string,
	agentID string,
	pipeRole string,
	coreVersion string,
	metadata *pb.HandshakeMetadata,
	mappings []*domain.SymbolMapping,
	previous *handshake.Evaluation,
) (*handshake.Evaluation, bool, error) {
	if e.protocolValidator == nil {
		return nil, false, fmt.Errorf("protocol validator no inicializado")
	}
	if e.repository == nil {
		return nil, false, fmt.Errorf("handshake repository no inicializado")
	}

	ctx, span := e.startSpan(ctx, "core.handshake.evaluate")
	defer span.End()

	if e.symbolSpecService != nil {
		e.symbolSpecService.WarmAccount(ctx, accountID)
	}

	prev := previous
	if prev == nil {
		loaded, err := e.repository.GetLatestByAccount(ctx, accountID)
		if err != nil {
			if e.telemetry != nil {
				e.telemetry.RecordError(ctx, err)
			}
			return nil, false, fmt.Errorf("obtener evaluación previa: %w", err)
		}
		prev = loaded
	}

	if isMaster(pipeRole) {
		evaluation := e.buildMasterEvaluation(accountID, agentID, pipeRole, coreVersion, metadata, mappings)

		if handshake.Equivalent(evaluation, prev) && prev != nil {
			prev.AgentID = agentID
			prev.CoreVersion = coreVersion
			prev.PipeRole = pipeRole
			prev.ProtocolVersion = evaluation.ProtocolVersion
			prev.ClientSemver = evaluation.ClientSemver
			prev.RequiredFeatures = evaluation.RequiredFeatures
			prev.OptionalFeatures = evaluation.OptionalFeatures
			prev.Capabilities = evaluation.Capabilities

			if e.telemetry != nil {
				status := registrationStatusString(prev.Status)
				if status != "ACCEPTED" {
					e.telemetry.Debug(ctx, "Handshake evaluation unchanged (master)",
						semconv.Echo.AccountID.String(accountID),
						attribute.String("pipe_role", pipeRole),
						attribute.String("status", status),
					)
				} else {
					e.telemetry.Info(ctx, "Handshake evaluation unchanged (master)",
						semconv.Echo.AccountID.String(accountID),
						attribute.String("pipe_role", pipeRole),
						attribute.String("status", status),
					)
				}
			}

			return prev, false, nil
		}

		if err := e.repository.CreateEvaluation(ctx, evaluation); err != nil {
			if e.telemetry != nil {
				e.telemetry.RecordError(ctx, err)
			}
			return nil, false, fmt.Errorf("persist evaluation: %w", err)
		}

		e.recordEvaluationMetrics(ctx, evaluation)
		e.logEvaluation(ctx, evaluation)

		return evaluation, true, nil
	}

	evaluation := e.buildEvaluation(ctx, accountID, agentID, pipeRole, coreVersion, metadata, mappings)

	if handshake.Equivalent(evaluation, prev) && prev != nil {
		prev.AgentID = agentID
		prev.CoreVersion = coreVersion
		prev.PipeRole = pipeRole
		prev.ProtocolVersion = evaluation.ProtocolVersion
		prev.ClientSemver = evaluation.ClientSemver
		prev.RequiredFeatures = evaluation.RequiredFeatures
		prev.OptionalFeatures = evaluation.OptionalFeatures
		prev.Capabilities = evaluation.Capabilities

		if e.telemetry != nil {
			status := registrationStatusString(prev.Status)
			if status != "ACCEPTED" {
				e.telemetry.Debug(ctx, "Handshake evaluation unchanged",
					semconv.Echo.AccountID.String(accountID),
					attribute.String("pipe_role", pipeRole),
					attribute.String("status", status),
				)
			} else {
				e.telemetry.Info(ctx, "Handshake evaluation unchanged",
				semconv.Echo.AccountID.String(accountID),
				attribute.String("pipe_role", pipeRole),
					attribute.String("status", status),
			)
			}
		}
		return prev, false, nil
	}

	if err := e.repository.CreateEvaluation(ctx, evaluation); err != nil {
		if e.telemetry != nil {
			e.telemetry.RecordError(ctx, err)
		}
		return nil, false, fmt.Errorf("persist evaluation: %w", err)
	}

	e.recordEvaluationMetrics(ctx, evaluation)
	e.logEvaluation(ctx, evaluation)

	return evaluation, true, nil
}

func (e *HandshakeEvaluator) buildEvaluation(
	ctx context.Context,
	accountID string,
	agentID string,
	pipeRole string,
	coreVersion string,
	metadata *pb.HandshakeMetadata,
	mappings []*domain.SymbolMapping,
) *handshake.Evaluation {
	if metadata == nil {
		metadata = &pb.HandshakeMetadata{}
	}

	evaluation := handshake.NewEvaluation(accountID, pipeRole)
	evaluation.AgentID = agentID
	evaluation.CoreVersion = coreVersion
	evaluation.ProtocolVersion = int(metadata.GetProtocolVersion())
	evaluation.ClientSemver = metadata.GetClientSemver()
	evaluation.RequiredFeatures = append([]string(nil), metadata.GetRequiredFeatures()...)
	evaluation.OptionalFeatures = append([]string(nil), metadata.GetOptionalFeatures()...)
	if metadata.GetCapabilities() != nil {
		evaluation.Capabilities = handshake.CapabilitySet{
			Features: append([]string(nil), metadata.Capabilities.GetFeatures()...),
			Metrics:  append([]string(nil), metadata.Capabilities.GetMetrics()...),
		}
	}

	status, protoErrors, protoWarnings := e.protocolValidator.Validate(metadata)
	evaluation.Status = status
	evaluation.Errors = append(evaluation.Errors, protoErrors...)
	evaluation.Warnings = append(evaluation.Warnings, protoWarnings...)

	specMaxAge := e.specMaxAge
	if specMaxAge <= 0 {
		specMaxAge = 10 * time.Second
	}

	for _, mapping := range mappings {
		if mapping == nil {
			continue
		}
		entry := handshake.Entry{
			CanonicalSymbol: mapping.CanonicalSymbol,
			BrokerSymbol:    mapping.BrokerSymbol,
			Status:          handshake.RegistrationStatusAccepted,
			EvaluatedAtMs:   evaluation.EvaluatedAtMs,
		}

		if !e.canonicalValidator.IsValid(mapping.CanonicalSymbol) {
			entry.Status = handshake.RegistrationStatusRejected
			entry.Errors = append(entry.Errors, handshake.Issue{
				Code:    handshake.IssueCodeCanonicalNotAllowed,
				Message: fmt.Sprintf("Símbolo canónico %s no permitido", mapping.CanonicalSymbol),
				Metadata: map[string]string{
					"canonical_symbol": mapping.CanonicalSymbol,
				},
			})
		}

		specAge, specFound := e.symbolSpecService.SpecAge(accountID, mapping.CanonicalSymbol)
		if specFound {
			entry.SpecAgeMs = specAge.Milliseconds()
			if specMaxAge > 0 && specAge > specMaxAge {
				entry.Warnings = append(entry.Warnings, handshake.Issue{
					Code:    handshake.IssueCodeSpecStale,
					Message: "Spec de antigüedad supera umbral configurado",
					Metadata: map[string]string{
						"threshold_ms": fmt.Sprintf("%d", specMaxAge.Milliseconds()),
					},
				})
				if entry.Status == handshake.RegistrationStatusAccepted {
					entry.Status = handshake.RegistrationStatusWarning
				}
			}
		} else {
			entry.Errors = append(entry.Errors, handshake.Issue{
				Code:    handshake.IssueCodeSpecStale,
				Message: "Spec no encontrada para símbolo",
				Metadata: map[string]string{
					"canonical_symbol": mapping.CanonicalSymbol,
				},
			})
			entry.Status = handshake.RegistrationStatusRejected
		}

		policy, policyErr := e.riskPolicyService.Get(ctx, accountID, "default")
		if policyErr != nil || policy == nil {
			entry.Errors = append(entry.Errors, handshake.Issue{
				Code:    handshake.IssueCodeRiskPolicyMissing,
				Message: "Política de riesgo faltante",
				Metadata: map[string]string{
					"account_id": accountID,
				},
			})
			entry.Status = handshake.RegistrationStatusRejected
		} else if policy.Type == domain.RiskPolicyTypeFixedLot {
			if policy.FixedLot == nil || policy.FixedLot.LotSize <= 0 {
				entry.Errors = append(entry.Errors, handshake.Issue{
					Code:    handshake.IssueCodeRiskPolicyMissing,
					Message: "Política FIXED_LOT sin lot_size válido",
					Metadata: map[string]string{
						"account_id": accountID,
					},
				})
				entry.Status = handshake.RegistrationStatusRejected
			}
		}

		supportsTick := evaluation.Capabilities.Supports("spec_report/tickvalue") ||
			handshake.Supports(evaluation.RequiredFeatures, "spec_report/tickvalue") ||
			handshake.Supports(evaluation.OptionalFeatures, "spec_report/tickvalue")

		if policy != nil && policy.Type == domain.RiskPolicyTypeFixedRisk {
			if !supportsTick {
				entry.Errors = append(entry.Errors, handshake.Issue{
					Code:    handshake.IssueCodeFeatureMissing,
					Message: "Capability spec_report/tickvalue requerida para FIXED_RISK",
					Metadata: map[string]string{
						"account_id": accountID,
					},
				})
				entry.Status = handshake.RegistrationStatusRejected
			}
		}

		evaluation.Status = mergeStatus(evaluation.Status, entry.Status)
		evaluation.Entries = append(evaluation.Entries, entry)
	}

	return evaluation
}

func (e *HandshakeEvaluator) recordEvaluationMetrics(ctx context.Context, evaluation *handshake.Evaluation) {
	if e.metrics == nil || evaluation == nil {
		return
	}

	statusStr := registrationStatusString(evaluation.Status)
	e.metrics.RecordHandshakeVersion(ctx, evaluation.ProtocolVersion, statusStr,
		semconv.Echo.AccountID.String(evaluation.AccountID),
		attribute.String("pipe_role", evaluation.PipeRole),
		attribute.String("client_semver", evaluation.ClientSemver),
	)

	for _, issue := range evaluation.Errors {
		e.metrics.RecordSymbolRegistrationIssue(ctx, issue.Code.String(), "global", "",
			semconv.Echo.AccountID.String(evaluation.AccountID),
			attribute.String("pipe_role", evaluation.PipeRole),
		)
	}
	for _, issue := range evaluation.Warnings {
		e.metrics.RecordSymbolRegistrationIssue(ctx, issue.Code.String(), "global", "",
			semconv.Echo.AccountID.String(evaluation.AccountID),
			attribute.String("pipe_role", evaluation.PipeRole),
		)
	}

	for _, entry := range evaluation.Entries {
		statusEntry := registrationStatusString(entry.Status)
		e.metrics.RecordSymbolRegistration(ctx, statusEntry, entry.CanonicalSymbol,
			semconv.Echo.AccountID.String(evaluation.AccountID),
			semconv.Echo.Symbol.String(entry.CanonicalSymbol),
		)
		for _, issue := range entry.Errors {
			e.metrics.RecordSymbolRegistrationIssue(ctx, issue.Code.String(), "symbol", entry.CanonicalSymbol,
				semconv.Echo.AccountID.String(evaluation.AccountID),
				semconv.Echo.Symbol.String(entry.CanonicalSymbol),
			)
		}
		for _, issue := range entry.Warnings {
			e.metrics.RecordSymbolRegistrationIssue(ctx, issue.Code.String(), "symbol", entry.CanonicalSymbol,
				semconv.Echo.AccountID.String(evaluation.AccountID),
				semconv.Echo.Symbol.String(entry.CanonicalSymbol),
			)
		}
	}
}

func (e *HandshakeEvaluator) logEvaluation(ctx context.Context, evaluation *handshake.Evaluation) {
	if e.telemetry == nil || evaluation == nil {
		return
	}

	status := registrationStatusString(evaluation.Status)
	attrs := []attribute.KeyValue{
		semconv.Echo.AccountID.String(evaluation.AccountID),
		attribute.String("pipe_role", evaluation.PipeRole),
		attribute.String("status", registrationStatusString(evaluation.Status)),
		attribute.Int("symbols", len(evaluation.Entries)),
		attribute.String("client_semver", evaluation.ClientSemver),
		attribute.Int("protocol_version", evaluation.ProtocolVersion),
	}

	if codes := collectIssueCodes(evaluation.Errors); len(codes) > 0 {
		attrs = append(attrs, attribute.String("global_error_codes", strings.Join(codes, ",")))
	}
	if codes := collectIssueCodes(evaluation.Warnings); len(codes) > 0 {
		attrs = append(attrs, attribute.String("global_warning_codes", strings.Join(codes, ",")))
	}

	if entryCodes := collectEntryIssueSummary(evaluation.Entries); entryCodes != "" {
		attrs = append(attrs, attribute.String("symbol_issue_codes", entryCodes))
	}

	e.telemetry.Debug(ctx, "Handshake evaluado", attrs...)
	if status != "ACCEPTED" {
		e.telemetry.Debug(ctx, "Handshake evaluado", attrs...)
	} else {
		e.telemetry.Info(ctx, "Handshake evaluado", attrs...)
	}
}

func (e *HandshakeEvaluator) startSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	if e.telemetry != nil {
		return e.telemetry.StartSpan(ctx, name)
	}
	return ctx, trace.SpanFromContext(ctx)
}

func (e *HandshakeEvaluator) buildMasterEvaluation(
	accountID string,
	agentID string,
	pipeRole string,
	coreVersion string,
	metadata *pb.HandshakeMetadata,
	mappings []*domain.SymbolMapping,
) *handshake.Evaluation {
	evaluation := handshake.NewEvaluation(accountID, pipeRole)
	if metadata == nil {
		metadata = &pb.HandshakeMetadata{}
	}

	evaluation.AgentID = agentID
	evaluation.CoreVersion = coreVersion
	evaluation.ProtocolVersion = int(metadata.GetProtocolVersion())
	evaluation.ClientSemver = metadata.GetClientSemver()
	evaluation.RequiredFeatures = append([]string(nil), metadata.GetRequiredFeatures()...)
	evaluation.OptionalFeatures = append([]string(nil), metadata.GetOptionalFeatures()...)
	if metadata.GetCapabilities() != nil {
		evaluation.Capabilities = handshake.CapabilitySet{
			Features: append([]string(nil), metadata.Capabilities.GetFeatures()...),
			Metrics:  append([]string(nil), metadata.Capabilities.GetMetrics()...),
		}
	}

	evaluation.Status = handshake.RegistrationStatusAccepted

	for _, mapping := range mappings {
		if mapping == nil {
			continue
		}
		evaluation.Entries = append(evaluation.Entries, handshake.Entry{
			CanonicalSymbol: mapping.CanonicalSymbol,
			BrokerSymbol:    mapping.BrokerSymbol,
			Status:          handshake.RegistrationStatusAccepted,
			EvaluatedAtMs:   evaluation.EvaluatedAtMs,
		})
	}

	return evaluation
}

func collectIssueCodes(issues []handshake.Issue) []string {
	if len(issues) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		code := issue.Code.String()
		if code == "" {
			continue
		}
		set[code] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for code := range set {
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}

func isMaster(pipeRole string) bool {
	return strings.EqualFold(pipeRole, "master")
}

func collectEntryIssueSummary(entries []handshake.Entry) string {
	if len(entries) == 0 {
		return ""
	}
	summary := make([]string, 0, len(entries))
	for _, entry := range entries {
		issues := collectIssueCodes(entry.Errors)
		if len(issues) == 0 {
			continue
		}
		summary = append(summary, fmt.Sprintf("%s:%s", entry.CanonicalSymbol, strings.Join(issues, "|")))
	}
	sort.Strings(summary)
	return strings.Join(summary, ",")
}

func mergeStatus(current, update handshake.RegistrationStatus) handshake.RegistrationStatus {
	if severity(update) > severity(current) {
		return update
	}
	return current
}

func severity(status handshake.RegistrationStatus) int {
	switch status {
	case handshake.RegistrationStatusRejected:
		return 3
	case handshake.RegistrationStatusWarning:
		return 2
	case handshake.RegistrationStatusAccepted:
		return 1
	default:
		return 0
	}
}

func registrationStatusString(status handshake.RegistrationStatus) string {
	switch status {
	case handshake.RegistrationStatusAccepted:
		return "ACCEPTED"
	case handshake.RegistrationStatusWarning:
		return "WARNING"
	case handshake.RegistrationStatusRejected:
		return "REJECTED"
	default:
		return "UNSPECIFIED"
	}
}
