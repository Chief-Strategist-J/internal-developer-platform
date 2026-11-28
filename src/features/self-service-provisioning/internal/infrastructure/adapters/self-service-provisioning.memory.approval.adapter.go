/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING ADAPTER (V.3 role 11, Invariant A)

 1. Role:
    - Concrete adapter implementation separated per V.4 Invariant A.
 2. Invariants:
    - Zero inline comments below this header.
*/
package adapters

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

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
