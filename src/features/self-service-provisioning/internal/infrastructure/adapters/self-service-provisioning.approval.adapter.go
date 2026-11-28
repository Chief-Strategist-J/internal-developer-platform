/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING APPROVAL ADAPTERS (V.3 role 11)

 1. Role:
    - Persists and verifies INV-09 approvals (approver != requester, expiry, aggregate period budget) and INV-10/EC-09 plan pinning.
 2. Invariants:
    - Uses RunTracedClient / WithClientSpan (SpanKindClient); atomic APPROVED -> CONSUMED transition.
    - Zero inline comments below this header.
*/
package adapters

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresProvisioningApprovalAdapter struct {
	db *sql.DB
}

func NewPostgresProvisioningApprovalAdapter(db *sql.DB) *PostgresProvisioningApprovalAdapter {
	return &PostgresProvisioningApprovalAdapter{db: db}
}

func (a *PostgresProvisioningApprovalAdapter) RecordApproval(ctx context.Context, approval contracts.ApprovalRecord) error {
	return observability.WithClientSpan(ctx, "self_service.approval.postgres.record", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning approval database connection is nil")
		}
		if strings.TrimSpace(approval.RequesterID) == "" || strings.TrimSpace(approval.ApproverID) == "" || strings.EqualFold(strings.TrimSpace(approval.RequesterID), strings.TrimSpace(approval.ApproverID)) {
			return errors.NewForbiddenError("self-approval is forbidden: approver must differ from requester")
		}
		publicID := approval.PublicID
		if publicID == "" {
			sum := sha256.Sum256([]byte(approval.ID + ":" + approval.TenantID + ":" + approval.PlanHash))
			publicID = "sspa_" + hex.EncodeToString(sum[:13])
		}
		budget := approval.BudgetUnits
		if budget <= 0 {
			budget = 1
		}
		status := approval.Status
		if status == "" {
			status = "APPROVED"
		}
		const stmt = `
			INSERT INTO self_service_provisioning.provisioning_approvals (
				id, public_id, tenant_id, requester_id, approver_id, plan_hash, template_sha, policy_sha, budget_units, status, expires_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`
		_, err := a.db.ExecContext(ctx, stmt, approval.ID, publicID, approval.TenantID, approval.RequesterID, approval.ApproverID, approval.PlanHash, approval.TemplateSHA, approval.PolicySHA, budget, status, approval.ExpiresAt)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("approval.id", approval.ID))
}

func (a *PostgresProvisioningApprovalAdapter) VerifyAndConsumeApproval(ctx context.Context, approvalID string, expectedPlan contracts.PinnedPlan, maxPeriodBudget int) error {
	return observability.WithClientSpan(ctx, "self_service.approval.postgres.verify_consume", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning approval database connection is nil")
		}
		tx, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin approval transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		var rec contracts.ApprovalRecord
		const selectStmt = `
			SELECT id, public_id, tenant_id, requester_id, approver_id, plan_hash, template_sha, policy_sha, budget_units, status, expires_at
			FROM self_service_provisioning.provisioning_approvals
			WHERE id = $1
			FOR UPDATE
		`
		if err := tx.QueryRowContext(ctx, selectStmt, approvalID).Scan(
			&rec.ID, &rec.PublicID, &rec.TenantID, &rec.RequesterID, &rec.ApproverID,
			&rec.PlanHash, &rec.TemplateSHA, &rec.PolicySHA, &rec.BudgetUnits, &rec.Status, &rec.ExpiresAt,
		); err != nil {
			if stderrors.Is(err, sql.ErrNoRows) {
				return errors.NewNotFoundError("approval record not found")
			}
			return err
		}

		var periodUsed int
		const budgetStmt = `
			SELECT COALESCE(SUM(budget_units), 0)
			FROM self_service_provisioning.provisioning_approvals
			WHERE tenant_id = $1 AND status = 'CONSUMED' AND created_at >= NOW() - INTERVAL '30 days'
		`
		if err := tx.QueryRowContext(ctx, budgetStmt, rec.TenantID).Scan(&periodUsed); err != nil {
			return err
		}

		if err := rules.ValidateApprovalAndPin(rec, expectedPlan, periodUsed, maxPeriodBudget, time.Now()); err != nil {
			return err
		}

		const consumeStmt = `
			UPDATE self_service_provisioning.provisioning_approvals
			SET status = 'CONSUMED', updated_at = NOW()
			WHERE id = $1 AND status = 'APPROVED'
		`
		if _, err := tx.ExecContext(ctx, consumeStmt, approvalID); err != nil {
			return err
		}
		return tx.Commit()
	}, attribute.String("db.system", "postgresql"), attribute.String("approval.id", approvalID))
}

type MemoryProvisioningApprovalAdapter struct {
	mu        sync.Mutex
	approvals map[string]contracts.ApprovalRecord
}

func NewMemoryProvisioningApprovalAdapter() *MemoryProvisioningApprovalAdapter {
	return &MemoryProvisioningApprovalAdapter{
		approvals: make(map[string]contracts.ApprovalRecord),
	}
}

func (a *MemoryProvisioningApprovalAdapter) RecordApproval(ctx context.Context, approval contracts.ApprovalRecord) error {
	return observability.WithClientSpan(ctx, "self_service.approval.memory.record", func(ctx context.Context) error {
		if strings.TrimSpace(approval.RequesterID) == "" || strings.TrimSpace(approval.ApproverID) == "" || strings.EqualFold(strings.TrimSpace(approval.RequesterID), strings.TrimSpace(approval.ApproverID)) {
			return errors.NewForbiddenError("self-approval is forbidden: approver must differ from requester")
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if approval.BudgetUnits <= 0 {
			approval.BudgetUnits = 1
		}
		if approval.Status == "" {
			approval.Status = "APPROVED"
		}
		a.approvals[approval.ID] = approval
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("approval.id", approval.ID))
}

func (a *MemoryProvisioningApprovalAdapter) VerifyAndConsumeApproval(ctx context.Context, approvalID string, expectedPlan contracts.PinnedPlan, maxPeriodBudget int) error {
	return observability.WithClientSpan(ctx, "self_service.approval.memory.verify_consume", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		rec, ok := a.approvals[approvalID]
		if !ok {
			return errors.NewNotFoundError("approval record not found")
		}
		periodUsed := 0
		for _, item := range a.approvals {
			if item.TenantID == rec.TenantID && item.Status == "CONSUMED" {
				periodUsed += item.BudgetUnits
			}
		}
		if err := rules.ValidateApprovalAndPin(rec, expectedPlan, periodUsed, maxPeriodBudget, time.Now()); err != nil {
			return err
		}
		rec.Status = "CONSUMED"
		a.approvals[approvalID] = rec
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("approval.id", approvalID))
}
