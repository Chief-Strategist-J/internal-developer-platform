/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC AUDIT OUTBOX ADAPTERS

1. Architecture Role:
  - Implements ports.RBACAuditOutboxPort for durable RBAC decision audit logging.
  - PostgresRBACAuditOutboxAdapter persists PERMIT, DENY, and ELEVATED_PERMIT events into rbac_audit_outbox.
  - MemoryRBACAuditOutboxAdapter provides a thread-safe in-memory outbox for unit tests and non-DB mode.
  - Enforces Tier-4 OpenTelemetry spans and zero inline comments inside function bodies.
*/
package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
	rbacQueries "internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/queries"
	"internal-developer-platform/src/infra/db"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresRBACAuditOutboxAdapter struct {
	db *sql.DB
}

func NewPostgresRBACAuditOutboxAdapter(db *sql.DB) *PostgresRBACAuditOutboxAdapter {
	return &PostgresRBACAuditOutboxAdapter{db: db}
}

func (a *PostgresRBACAuditOutboxAdapter) EnqueueAuditEvent(ctx context.Context, event rbacContracts.RBACAuditEvent) error {
	return observability.WithSpan(ctx, "postgres.rbac_audit_outbox.enqueue", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("database connection is nil for rbac audit outbox")
		}
		if event.ID == "" {
			return errors.NewValidationError("audit event id is required")
		}
		occurredAt := event.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		sum := sha256.Sum256([]byte(event.ID + ":" + event.TenantID))
		publicID := "raud_" + hex.EncodeToString(sum[:16])
		var err error
		if tx, ok := TxFromContext(ctx); ok {
			_, err = tx.ExecContext(
				ctx,
				rbacQueries.FlowInsertRBACAuditOutbox,
				event.ID,
				event.TenantID,
				event.SubjectID,
				string(event.Resource),
				string(event.Action),
				event.Decision,
				event.Reason,
				event.ViolationCode,
				occurredAt,
				publicID,
			)
		} else {
			featureTx, beginErr := db.BeginFeatureTx(ctx, a.db, db.FeatureTxScope{
				Schema:   "role_based_access_control",
				Role:     "svc_rbac",
				TenantID: event.TenantID,
			})
			if beginErr != nil {
				return fmt.Errorf("failed to begin rbac audit outbox transaction: %w", beginErr)
			}
			defer featureTx.Rollback()
			_, err = featureTx.ExecContext(
				ctx,
				rbacQueries.FlowInsertRBACAuditOutbox,
				event.ID,
				event.TenantID,
				event.SubjectID,
				string(event.Resource),
				string(event.Action),
				event.Decision,
				event.Reason,
				event.ViolationCode,
				occurredAt,
				publicID,
			)
			if err == nil {
				err = featureTx.Commit()
			}
		}
		if err != nil {
			return fmt.Errorf("failed to enqueue rbac audit outbox event: %w", err)
		}
		return nil
	}, attribute.String("event.id", event.ID), attribute.String("decision", event.Decision), attribute.String("subject.id", event.SubjectID))
}

func (a *PostgresRBACAuditOutboxAdapter) ListPendingAuditEvents(ctx context.Context, limit int) ([]rbacContracts.RBACAuditEvent, error) {
	return observability.RunTraced(ctx, "postgres.rbac_audit_outbox.list_pending", func(ctx context.Context) ([]rbacContracts.RBACAuditEvent, error) {
		if a.db == nil {
			return nil, errors.NewInternalError("database connection is nil for rbac audit outbox")
		}
		if limit <= 0 {
			limit = 50
		}
		rows, err := a.db.QueryContext(ctx, rbacQueries.FLOW_LIST_PENDING_RBAC_AUDIT_OUTBOX, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to list pending rbac audit outbox events: %w", err)
		}
		defer rows.Close()
		var events []rbacContracts.RBACAuditEvent
		for rows.Next() {
			var ev rbacContracts.RBACAuditEvent
			var res, act string
			if err := rows.Scan(&ev.ID, &ev.TenantID, &ev.SubjectID, &res, &act, &ev.Decision, &ev.Reason, &ev.ViolationCode, &ev.OccurredAt); err != nil {
				return nil, fmt.Errorf("failed to scan rbac audit outbox event: %w", err)
			}
			ev.Resource = rbacContracts.Resource(res)
			ev.Action = rbacContracts.Action(act)
			events = append(events, ev)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows iteration error: %w", err)
		}
		return events, nil
	})
}

func (a *PostgresRBACAuditOutboxAdapter) MarkAuditEventPublished(ctx context.Context, id string) error {
	return observability.WithSpan(ctx, "postgres.rbac_audit_outbox.mark_published", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("database connection is nil for rbac audit outbox")
		}
		_, err := a.db.ExecContext(ctx, rbacQueries.FLOW_MARK_RBAC_AUDIT_OUTBOX_PUBLISHED, id, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("failed to mark rbac audit outbox event published: %w", err)
		}
		return nil
	}, attribute.String("event.id", id))
}

type MemoryRBACAuditOutboxAdapter struct {
	mu     sync.RWMutex
	events []rbacContracts.RBACAuditEvent
}

func NewMemoryRBACAuditOutboxAdapter() *MemoryRBACAuditOutboxAdapter {
	return &MemoryRBACAuditOutboxAdapter{
		events: make([]rbacContracts.RBACAuditEvent, 0),
	}
}

func (a *MemoryRBACAuditOutboxAdapter) EnqueueAuditEvent(ctx context.Context, event rbacContracts.RBACAuditEvent) error {
	return observability.WithSpan(ctx, "memory.rbac_audit_outbox.enqueue", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.events = append(a.events, event)
		return nil
	}, attribute.String("event.id", event.ID), attribute.String("decision", event.Decision))
}

func (a *MemoryRBACAuditOutboxAdapter) ListEvents() []rbacContracts.RBACAuditEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	copied := make([]rbacContracts.RBACAuditEvent, len(a.events))
	copy(copied, a.events)
	return copied
}

type HTTPBreakGlassTicketAdapter struct {
	pagerDutyEndpoint string
	pagerDutyToken    string
	jiraEndpoint      string
	jiraToken         string
	httpClient        *http.Client
}

func NewHTTPBreakGlassTicketAdapter(pdEndpoint string, pdToken string, jiraEndpoint string, jiraToken string) *HTTPBreakGlassTicketAdapter {
	return &HTTPBreakGlassTicketAdapter{
		pagerDutyEndpoint: strings.TrimRight(pdEndpoint, "/"),
		pagerDutyToken:    pdToken,
		jiraEndpoint:      strings.TrimRight(jiraEndpoint, "/"),
		jiraToken:         jiraToken,
		httpClient:        &http.Client{Timeout: 5 * time.Second},
	}
}

type pagerDutyIncidentResponse struct {
	Incident struct {
		Status string `json:"status"`
	} `json:"incident"`
}

type jiraIssueResponse struct {
	Fields struct {
		Status struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

func (a *HTTPBreakGlassTicketAdapter) VerifyTicketApproval(ctx context.Context, ticketID string) (bool, error) {
	return observability.RunTraced(ctx, "http.rbac.verify_breakglass_ticket", func(ctx context.Context) (bool, error) {
		if ticketID == "" {
			return false, errors.NewValidationError("break-glass ticketId is required")
		}
		targetURL := ""
		authHeader := ""
		isPagerDuty := false
		if strings.HasPrefix(strings.ToUpper(ticketID), "PD-") && a.pagerDutyEndpoint != "" && a.pagerDutyToken != "" {
			isPagerDuty = true
			normalizedID := ticketID[3:]
			if normalizedID == "" {
				return false, errors.NewValidationError("break-glass pagerduty incident id is required")
			}
			targetURL = fmt.Sprintf("%s/incidents/%s", a.pagerDutyEndpoint, url.PathEscape(normalizedID))
			authHeader = "Token token=" + a.pagerDutyToken
		} else if a.jiraEndpoint != "" && a.jiraToken != "" {
			targetURL = fmt.Sprintf("%s/rest/api/3/issue/%s", a.jiraEndpoint, url.PathEscape(ticketID))
			authHeader = "Bearer " + a.jiraToken
		}
		if targetURL == "" {
			return false, errors.NewDownstreamUnavailableError("break-glass ticket verification provider is not configured")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return false, fmt.Errorf("failed to build ticket verification request: %w", err)
		}
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Accept", "application/json")
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return false, fmt.Errorf("ticket verification request failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return false, nil
		}
		if isPagerDuty {
			var pdResp pagerDutyIncidentResponse
			if err := json.NewDecoder(resp.Body).Decode(&pdResp); err != nil {
				return false, fmt.Errorf("failed to decode pagerduty incident response: %w", err)
			}
			status := strings.ToLower(strings.TrimSpace(pdResp.Incident.Status))
			return status == "triggered" || status == "acknowledged", nil
		}
		var jiraResp jiraIssueResponse
		if err := json.NewDecoder(resp.Body).Decode(&jiraResp); err != nil {
			return false, fmt.Errorf("failed to decode jira issue response: %w", err)
		}
		status := strings.ToLower(strings.TrimSpace(jiraResp.Fields.Status.Name))
		return status == "approved" || status == "in progress" || status == "active" || status == "open" || status == "escalated", nil
	}, attribute.String("ticket.id", ticketID))
}

type HTTPSIEMAlertAdapter struct {
	webhookURL string
	apiToken   string
	httpClient *http.Client
}

func NewHTTPSIEMAlertAdapter(webhookURL string, apiToken string) *HTTPSIEMAlertAdapter {
	return &HTTPSIEMAlertAdapter{
		webhookURL: strings.TrimSpace(webhookURL),
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (a *HTTPSIEMAlertAdapter) EmitSecurityAlert(ctx context.Context, event rbacContracts.RBACAuditEvent) error {
	return observability.WithSpan(ctx, "http.rbac.emit_siem_alert", func(ctx context.Context) error {
		if a.webhookURL == "" {
			return nil
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal siem alert event: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to build siem alert request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if a.apiToken != "" {
			req.Header.Set("Authorization", "Bearer "+a.apiToken)
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("siem alert dispatch failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("siem webhook returned status %d", resp.StatusCode)
		}
		return nil
	}, attribute.String("event.id", event.ID), attribute.String("decision", event.Decision))
}

type HTTPOIDCIntrospectorAdapter struct {
	endpoint     string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewHTTPOIDCIntrospectorAdapter(endpoint string, clientID string, clientSecret string) *HTTPOIDCIntrospectorAdapter {
	return &HTTPOIDCIntrospectorAdapter{
		endpoint:     strings.TrimSpace(endpoint),
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (a *HTTPOIDCIntrospectorAdapter) IntrospectToken(ctx context.Context, rawToken string) (bool, error) {
	return observability.RunTraced(ctx, "http.rbac.oidc_introspect", func(ctx context.Context) (bool, error) {
		if a.endpoint == "" {
			return false, errors.NewDownstreamUnavailableError("oidc introspection endpoint is not configured")
		}
		form := url.Values{}
		form.Set("token", rawToken)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return false, fmt.Errorf("failed to build oidc introspect request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if a.clientID != "" {
			req.SetBasicAuth(a.clientID, a.clientSecret)
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return false, fmt.Errorf("oidc introspect request failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return false, fmt.Errorf("oidc introspect returned status %d", resp.StatusCode)
		}
		var payload struct {
			Active bool `json:"active"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return false, fmt.Errorf("failed to decode oidc introspect response: %w", err)
		}
		return payload.Active, nil
	})
}
