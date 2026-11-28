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
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/queries"
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
		_, err := a.db.ExecContext(ctx, queries.InsertProvisioningApprovalSQL, approval.ID, publicID, approval.TenantID, approval.RequesterID, approval.ApproverID, approval.PlanHash, approval.TemplateSHA, approval.PolicySHA, budget, status, approval.ExpiresAt)
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
		if err := tx.QueryRowContext(ctx, queries.SelectApprovalForUpdateSQL, approvalID).Scan(
			&rec.ID, &rec.PublicID, &rec.TenantID, &rec.RequesterID, &rec.ApproverID,
			&rec.PlanHash, &rec.TemplateSHA, &rec.PolicySHA, &rec.BudgetUnits, &rec.Status, &rec.ExpiresAt,
		); err != nil {
			if stderrors.Is(err, sql.ErrNoRows) {
				return errors.NewNotFoundError("approval record not found")
			}
			return err
		}

		var periodUsed int
		if err := tx.QueryRowContext(ctx, queries.SumConsumedApprovalBudget30dSQL, rec.TenantID).Scan(&periodUsed); err != nil {
			return err
		}

		if err := rules.ValidateApprovalAndPin(rec, expectedPlan, periodUsed, maxPeriodBudget, time.Now()); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, queries.ConsumeProvisioningApprovalSQL, approvalID); err != nil {
			return err
		}
		return tx.Commit()
	}, attribute.String("db.system", "postgresql"), attribute.String("approval.id", approvalID))
}
