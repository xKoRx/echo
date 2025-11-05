package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xKoRx/echo/sdk/domain/handshake"
	"github.com/xKoRx/echo/sdk/utils"
)

type postgresHandshakeRepo struct {
	db *sql.DB
}

func (r *postgresHandshakeRepo) CreateEvaluation(ctx context.Context, evaluation *handshake.Evaluation) error {
	if evaluation == nil {
		return fmt.Errorf("nil evaluation")
	}

	if evaluation.EvaluationID == "" {
		evaluation.EvaluationID = utils.GenerateUUIDv7()
	}
	if evaluation.EvaluatedAtMs == 0 {
		evaluation.EvaluatedAtMs = time.Now().UnixMilli()
	}

	errorsJSON, err := marshalIssues(evaluation.Errors)
	if err != nil {
		return fmt.Errorf("marshal global errors: %w", err)
	}
	warningsJSON, err := marshalIssues(evaluation.Warnings)
	if err != nil {
		return fmt.Errorf("marshal global warnings: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	evaluatedAt := time.UnixMilli(evaluation.EvaluatedAtMs)

	requiredJSON, err := json.Marshal(evaluation.RequiredFeatures)
	if err != nil {
		return fmt.Errorf("marshal required features: %w", err)
	}
	optionalJSON, err := json.Marshal(evaluation.OptionalFeatures)
	if err != nil {
		return fmt.Errorf("marshal optional features: %w", err)
	}
	capabilitiesJSON, err := json.Marshal(map[string]interface{}{
		"features": evaluation.Capabilities.Features,
		"metrics":  evaluation.Capabilities.Metrics,
	})
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}

	insertEval := `
        INSERT INTO echo.account_symbol_registration_eval (
            evaluation_id, account_id, pipe_role, status,
            protocol_version, client_semver, global_errors,
            global_warnings, required_features, optional_features,
            capabilities, evaluated_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11::jsonb,$12)
    `

	if _, err = tx.ExecContext(
		ctx,
		insertEval,
		evaluation.EvaluationID,
		evaluation.AccountID,
		evaluation.PipeRole,
		statusToString(evaluation.Status),
		evaluation.ProtocolVersion,
		evaluation.ClientSemver,
		string(errorsJSON),
		string(warningsJSON),
		string(requiredJSON),
		string(optionalJSON),
		string(capabilitiesJSON),
		evaluatedAt,
	); err != nil {
		return fmt.Errorf("insert evaluation: %w", err)
	}

	insertEntry := `
        INSERT INTO echo.account_symbol_registration (
            evaluation_id, canonical_symbol, broker_symbol,
            status, warnings, errors, spec_age_ms, evaluated_at
        ) VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8)
    `

	for _, entry := range evaluation.Entries {
		warnJSON, errMarshal := marshalIssues(entry.Warnings)
		if errMarshal != nil {
			err = fmt.Errorf("marshal entry warnings: %w", errMarshal)
			return err
		}
		errJSON, errMarshal := marshalIssues(entry.Errors)
		if errMarshal != nil {
			err = fmt.Errorf("marshal entry errors: %w", errMarshal)
			return err
		}
		entryEvaluatedAt := evaluatedAt
		if entry.EvaluatedAtMs > 0 {
			entryEvaluatedAt = time.UnixMilli(entry.EvaluatedAtMs)
		}
		if _, err = tx.ExecContext(
			ctx,
			insertEntry,
			evaluation.EvaluationID,
			entry.CanonicalSymbol,
			entry.BrokerSymbol,
			statusToString(entry.Status),
			string(warnJSON),
			string(errJSON),
			entry.SpecAgeMs,
			entryEvaluatedAt,
		); err != nil {
			return fmt.Errorf("insert entry %s: %w", entry.CanonicalSymbol, err)
		}
	}

	notifyPayload := fmt.Sprintf("%s:%s", evaluation.AccountID, evaluation.EvaluationID)
	if _, err = tx.ExecContext(ctx, `SELECT pg_notify('echo_handshake_result', $1)`, notifyPayload); err != nil {
		return fmt.Errorf("notify handshake result: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit evaluation: %w", err)
	}

	return nil
}

func (r *postgresHandshakeRepo) GetLatestByAccount(ctx context.Context, accountID string) (*handshake.Evaluation, error) {
	queryEval := `
        SELECT evaluation_id, pipe_role, status, protocol_version, client_semver,
               global_errors, global_warnings, required_features, optional_features,
               capabilities, evaluated_at
        FROM echo.account_symbol_registration_eval
        WHERE account_id = $1
        ORDER BY evaluated_at DESC
        LIMIT 1
    `

	var (
		evalID           string
		pipeRole         string
		statusStr        string
		protocolVersion  int
		clientSemver     sql.NullString
		errorsJSON       []byte
		warningsJSON     []byte
		requiredJSON     []byte
		optionalJSON     []byte
		capabilitiesJSON []byte
		evaluatedAt      time.Time
	)

	err := r.db.QueryRowContext(ctx, queryEval, accountID).Scan(
		&evalID,
		&pipeRole,
		&statusStr,
		&protocolVersion,
		&clientSemver,
		&errorsJSON,
		&warningsJSON,
		&requiredJSON,
		&optionalJSON,
		&capabilitiesJSON,
		&evaluatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select evaluation: %w", err)
	}

	errorsIssues, err := unmarshalIssues(errorsJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal global errors: %w", err)
	}
	warningsIssues, err := unmarshalIssues(warningsJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal global warnings: %w", err)
	}

	evaluation := &handshake.Evaluation{
		EvaluationID:    evalID,
		AccountID:       accountID,
		PipeRole:        pipeRole,
		ProtocolVersion: protocolVersion,
		ClientSemver:    clientSemver.String,
		Status:          stringToStatus(statusStr),
		Errors:          errorsIssues,
		Warnings:        warningsIssues,
		EvaluatedAtMs:   evaluatedAt.UnixMilli(),
	}

	if err := json.Unmarshal(requiredJSON, &evaluation.RequiredFeatures); err != nil {
		return nil, fmt.Errorf("unmarshal required features: %w", err)
	}
	if err := json.Unmarshal(optionalJSON, &evaluation.OptionalFeatures); err != nil {
		return nil, fmt.Errorf("unmarshal optional features: %w", err)
	}
	var capabilitiesPayload map[string][]string
	if err := json.Unmarshal(capabilitiesJSON, &capabilitiesPayload); err == nil {
		evaluation.Capabilities = handshake.CapabilitySet{
			Features: capabilitiesPayload["features"],
			Metrics:  capabilitiesPayload["metrics"],
		}
	}

	queryEntries := `
        SELECT canonical_symbol, broker_symbol, status,
               warnings, errors, spec_age_ms, evaluated_at
        FROM echo.account_symbol_registration
        WHERE evaluation_id = $1
    `

	rows, err := r.db.QueryContext(ctx, queryEntries, evalID)
	if err != nil {
		return nil, fmt.Errorf("select entries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			canonical string
			broker    string
			status    string
			warnJSON  []byte
			errJSON   []byte
			specAge   int64
			entryEval time.Time
		)
		if err = rows.Scan(&canonical, &broker, &status, &warnJSON, &errJSON, &specAge, &entryEval); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		warnings, errWarn := unmarshalIssues(warnJSON)
		if errWarn != nil {
			return nil, fmt.Errorf("unmarshal entry warnings: %w", errWarn)
		}
		errorsIssues, errErr := unmarshalIssues(errJSON)
		if errErr != nil {
			return nil, fmt.Errorf("unmarshal entry errors: %w", errErr)
		}

		evaluation.Entries = append(evaluation.Entries, handshake.Entry{
			CanonicalSymbol: canonical,
			BrokerSymbol:    broker,
			Status:          stringToStatus(status),
			Warnings:        warnings,
			Errors:          errorsIssues,
			SpecAgeMs:       specAge,
			EvaluatedAtMs:   entryEval.UnixMilli(),
		})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return evaluation, nil
}

func marshalIssues(issues []handshake.Issue) ([]byte, error) {
	if len(issues) == 0 {
		return []byte("[]"), nil
	}
	type issueDTO struct {
		Code     string            `json:"code"`
		Message  string            `json:"message"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}
	payload := make([]issueDTO, 0, len(issues))
	for _, issue := range issues {
		payload = append(payload, issueDTO{
			Code:     issue.Code.String(),
			Message:  issue.Message,
			Metadata: issue.Metadata,
		})
	}
	return json.Marshal(payload)
}

func unmarshalIssues(data []byte) ([]handshake.Issue, error) {
	if len(data) == 0 {
		return nil, nil
	}
	type issueDTO struct {
		Code     string            `json:"code"`
		Message  string            `json:"message"`
		Metadata map[string]string `json:"metadata"`
	}
	var payload []issueDTO
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	issues := make([]handshake.Issue, 0, len(payload))
	for _, dto := range payload {
		issues = append(issues, handshake.Issue{
			Code:     stringToIssueCode(dto.Code),
			Message:  dto.Message,
			Metadata: dto.Metadata,
		})
	}
	return issues, nil
}

func statusToString(status handshake.RegistrationStatus) string {
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

func stringToStatus(value string) handshake.RegistrationStatus {
	switch value {
	case "ACCEPTED":
		return handshake.RegistrationStatusAccepted
	case "WARNING":
		return handshake.RegistrationStatusWarning
	case "REJECTED":
		return handshake.RegistrationStatusRejected
	default:
		return handshake.RegistrationStatusUnspecified
	}
}

func stringToIssueCode(value string) handshake.IssueCode {
	switch value {
	case handshake.IssueCodeCanonicalNotAllowed.String():
		return handshake.IssueCodeCanonicalNotAllowed
	case handshake.IssueCodeSpecStale.String():
		return handshake.IssueCodeSpecStale
	case handshake.IssueCodeRiskPolicyMissing.String():
		return handshake.IssueCodeRiskPolicyMissing
	case handshake.IssueCodeProtocolVersionUnsupported.String():
		return handshake.IssueCodeProtocolVersionUnsupported
	case handshake.IssueCodeFeatureMissing.String():
		return handshake.IssueCodeFeatureMissing
	default:
		return handshake.IssueCodeUnspecified
	}
}
