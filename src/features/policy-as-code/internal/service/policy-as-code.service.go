/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE DOMAIN SERVICE

1. Architecture Role:
  - Step 06 Service Orchestrator: Coordinates policy definitions, lifecycle FSM transitions, admission evaluation, and continuous inventory audit scans.
  - Enforces OpenTelemetry child spans for all domain workflows (Tier-3 spans under "policy.service.*").
  - Interacts strictly with outbound abstract port ports.PolicyRepositoryPort.
  - Storage-enforced invariant: Promotion to DENY validates active violations from database before transition.
  - Zero inline comments inside method bodies; strictly typed domain returns.
*/
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/policy-as-code/internal/contracts"
	"internal-developer-platform/src/features/policy-as-code/internal/domain/machines"
	"internal-developer-platform/src/features/policy-as-code/internal/domain/rules"
	"internal-developer-platform/src/features/policy-as-code/internal/domain/workflows"
	"internal-developer-platform/src/features/policy-as-code/internal/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PolicyAsCodeService struct {
	repo ports.PolicyRepositoryPort
}

func NewPolicyAsCodeService(repo ports.PolicyRepositoryPort) *PolicyAsCodeService {
	return &PolicyAsCodeService{
		repo: repo,
	}
}

func (s *PolicyAsCodeService) GetActiveExceptionForWorkload(ctx context.Context, tenantID, namespace, workloadName string) (*contracts.PolicyExceptionRecord, error) {
	if s.repo == nil {
		return nil, nil
	}
	exceptions, err := s.repo.GetActiveExceptionsForWorkload(ctx, tenantID, namespace, workloadName)
	if err != nil || len(exceptions) == 0 {
		return nil, err
	}
	matched := exceptions[0]
	return &matched, nil
}

func (s *PolicyAsCodeService) CreatePolicy(ctx context.Context, p contracts.PolicyRecord) (*contracts.PolicyRecord, error) {
	return observability.RunTraced(ctx, "policy.service.create_policy", func(ctx context.Context) (*contracts.PolicyRecord, error) {
		if err := rules.ValidateCELRuleExpression(p.RuleExpression); err != nil {
			return nil, err
		}

		if p.ID == "" {
			p.ID = uuid.New().String()
		}
		if p.PublicID == "" {
			p.PublicID = "pol_" + uuid.New().String()[:12]
		}
		if p.CorrelationID == "" {
			p.CorrelationID = uuid.New().String()
		}
		if p.Source == "" {
			p.Source = contracts.PolicySourceAPI
		}
		if p.LifecycleState == "" {
			p.LifecycleState = contracts.LifecycleDraft
		}

		now := time.Now().UTC()
		p.CreatedAt = now
		p.UpdatedAt = now
		p.Version = 1

		if err := p.Validate(); err != nil {
			return nil, err
		}

		if err := s.repo.SavePolicy(ctx, p); err != nil {
			return nil, err
		}

		return &p, nil
	}, attribute.String("policy.external_id", p.ExternalID), attribute.String("tenant.id", p.TenantID))
}

func (s *PolicyAsCodeService) GetPolicy(ctx context.Context, tenantID string, id string) (*contracts.PolicyRecord, error) {
	return observability.RunTraced(ctx, "policy.service.get_policy", func(ctx context.Context) (*contracts.PolicyRecord, error) {
		if tenantID == "" || id == "" {
			return nil, errors.NewValidationError("tenantId and id are required")
		}
		return s.repo.GetPolicyByID(ctx, tenantID, id)
	}, attribute.String("tenant.id", tenantID), attribute.String("policy.id", id))
}

func (s *PolicyAsCodeService) ListActivePolicies(ctx context.Context, tenantID string) ([]contracts.PolicyRecord, error) {
	return observability.RunTraced(ctx, "policy.service.list_active", func(ctx context.Context) ([]contracts.PolicyRecord, error) {
		if tenantID == "" {
			return nil, errors.NewValidationError("tenantId is required")
		}
		return s.repo.ListActiveCELPolicies(ctx, tenantID)
	}, attribute.String("tenant.id", tenantID))
}

func (s *PolicyAsCodeService) TransitionPolicyLifecycle(
	ctx context.Context,
	tenantID string,
	policyID string,
	event machines.PolicyLifecycleEvent,
	activeViolations int,
) (*contracts.PolicyRecord, error) {
	return observability.RunTraced(ctx, "policy.service.transition_lifecycle", func(ctx context.Context) (*contracts.PolicyRecord, error) {
		current, err := s.repo.GetPolicyByID(ctx, tenantID, policyID)
		if err != nil {
			return nil, err
		}

		if event == machines.EventPromoteToDeny && s.repo != nil {
			runningViolations, err := s.repo.CountActiveViolationsByPolicy(ctx, tenantID, policyID)
			if err == nil && runningViolations > 0 {
				activeViolations = runningViolations
			}
		}

		nextState, err := machines.TransitionPolicyLifecycle(current.LifecycleState, event, activeViolations)
		if err != nil {
			return nil, err
		}

		if err := s.repo.UpdatePolicyLifecycle(ctx, tenantID, policyID, nextState, current.Version); err != nil {
			return nil, err
		}

		current.LifecycleState = nextState
		current.Version++
		current.UpdatedAt = time.Now().UTC()
		return current, nil
	}, attribute.String("tenant.id", tenantID), attribute.String("policy.id", policyID), attribute.String("event", string(event)))
}

func (s *PolicyAsCodeService) RequestException(ctx context.Context, exc contracts.PolicyExceptionRecord) (*contracts.PolicyExceptionRecord, error) {
	return observability.RunTraced(ctx, "policy.service.request_exception", func(ctx context.Context) (*contracts.PolicyExceptionRecord, error) {
		policy, err := s.repo.GetPolicyByID(ctx, exc.TenantID, exc.PolicyID)
		if err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		if err := rules.ValidateExceptionEligibility(*policy, exc, now); err != nil {
			return nil, err
		}

		if exc.ID == "" {
			exc.ID = uuid.New().String()
		}
		if exc.PublicID == "" {
			exc.PublicID = "exc_" + uuid.New().String()[:12]
		}
		if exc.CorrelationID == "" {
			exc.CorrelationID = uuid.New().String()
		}
		if exc.Status == "" {
			exc.Status = contracts.ExceptionStatusActive
		}
		exc.CreatedAt = now
		exc.UpdatedAt = now
		exc.Version = 1

		if err := exc.Validate(); err != nil {
			return nil, err
		}

		if err := s.repo.SaveException(ctx, exc); err != nil {
			return nil, err
		}

		return &exc, nil
	}, attribute.String("tenant.id", exc.TenantID), attribute.String("policy.id", exc.PolicyID), attribute.String("workload.id", exc.WorkloadID))
}

func (s *PolicyAsCodeService) EvaluateAdmission(ctx context.Context, input workflows.AdmissionEvaluationInput) (workflows.AdmissionVerdict, error) {
	return observability.RunTraced(ctx, "policy.service.evaluate_admission", func(ctx context.Context) (workflows.AdmissionVerdict, error) {
		if input.TenantID == "" {
			return workflows.AdmissionVerdict{}, errors.NewValidationError("tenantId is required")
		}

		if (s.repo != nil) && (input.TenantID != "") {
			_, _ = s.repo.ReconcileExpiredExceptions(ctx, input.TenantID, time.Now().UTC())
		}

		if len(input.Policies) == 0 && s.repo != nil {
			policies, err := s.repo.ListActiveCELPolicies(ctx, input.TenantID)
			if err == nil {
				input.Policies = policies
			}
		}

		if len(input.Exceptions) == 0 && s.repo != nil && input.WorkloadID != "" {
			exceptions, err := s.repo.GetActiveExceptionsForWorkload(ctx, input.TenantID, input.Namespace, input.WorkloadID)
			if err == nil {
				input.Exceptions = exceptions
			}
		}

		if input.Timestamp.IsZero() {
			input.Timestamp = time.Now().UTC()
		}

		verdict := workflows.ExecuteAdmissionEvaluation(input)

		if (s.repo != nil) && (len(verdict.Violations) > 0) {
			for _, v := range verdict.Violations {
				if v.PolicyID == "" {
					continue
				}
				existingPol, err := s.repo.GetPolicyByID(ctx, input.TenantID, v.PolicyID)
				if err != nil || existingPol == nil {
					continue
				}
				if err := s.repo.SaveViolation(ctx, contracts.PolicyViolationRecord{
					ID:               uuid.New().String(),
					PublicID:         "viol_" + uuid.New().String()[:12],
					TenantID:         input.TenantID,
					PolicyID:         v.PolicyID,
					Source:           "ADMISSION",
					ExternalID:       fmt.Sprintf("%s:%s", v.ExternalID, input.WorkloadID),
					CorrelationID:    uuid.New().String(),
					WorkloadID:       input.WorkloadID,
					Namespace:        input.Namespace,
					Severity:         v.Severity,
					EnforcementPoint: v.Enforcement,
					Status:           contracts.ViolationStatusActive,
					Message:          v.Message,
					DetectionPair:    v.DetectionPair,
					Version:          1,
					CreatedAt:        input.Timestamp,
					UpdatedAt:        input.Timestamp,
				}); err != nil {
					observability.RecordError(observability.CurrentSpan(ctx), err)
				}
			}
		}

		return verdict, nil
	}, attribute.String("tenant.id", input.TenantID), attribute.String("workload.id", input.WorkloadID), attribute.String("namespace", input.Namespace))
}

func (s *PolicyAsCodeService) ListActiveViolations(ctx context.Context, tenantID, policyID string) ([]contracts.PolicyViolationRecord, error) {
	return observability.RunTraced(ctx, "policy.service.list_active_violations", func(ctx context.Context) ([]contracts.PolicyViolationRecord, error) {
		if tenantID == "" || policyID == "" {
			return nil, errors.NewValidationError("tenantId and policyId are required")
		}
		return s.repo.ListActiveViolationsByPolicy(ctx, tenantID, policyID)
	}, attribute.String("tenant.id", tenantID), attribute.String("policy.id", policyID))
}

func (s *PolicyAsCodeService) CountActiveViolations(ctx context.Context, tenantID, policyID string) (int, error) {
	return observability.RunTraced(ctx, "policy.service.count_active_violations", func(ctx context.Context) (int, error) {
		if tenantID == "" || policyID == "" {
			return 0, errors.NewValidationError("tenantId and policyId are required")
		}
		return s.repo.CountActiveViolationsByPolicy(ctx, tenantID, policyID)
	}, attribute.String("tenant.id", tenantID), attribute.String("policy.id", policyID))
}

func (s *PolicyAsCodeService) ResolveViolation(ctx context.Context, tenantID, violationID string, version int64) error {
	_, err := observability.RunTraced(ctx, "policy.service.resolve_violation", func(ctx context.Context) (any, error) {
		if tenantID == "" || violationID == "" {
			return nil, errors.NewValidationError("tenantId and violationId are required")
		}
		return nil, s.repo.ResolveViolation(ctx, tenantID, violationID, version)
	}, attribute.String("tenant.id", tenantID), attribute.String("violation.id", violationID))
	return err
}

func (s *PolicyAsCodeService) ExecuteAuditScan(ctx context.Context, input workflows.AuditScanInput) (*workflows.AuditScanReport, error) {
	return observability.RunTraced(ctx, "policy.service.execute_audit_scan", func(ctx context.Context) (*workflows.AuditScanReport, error) {
		if input.TenantID == "" {
			return nil, errors.NewValidationError("tenantId is required")
		}

		if (s.repo != nil) && (input.TenantID != "") {
			_, _ = s.repo.ReconcileExpiredExceptions(ctx, input.TenantID, time.Now().UTC())
		}

		if len(input.Policies) == 0 && s.repo != nil {
			policies, err := s.repo.ListActiveCELPolicies(ctx, input.TenantID)
			if err == nil {
				input.Policies = policies
			}
		}

		if len(input.Exceptions) == 0 && s.repo != nil {
			for _, w := range input.Workloads {
				if w.WorkloadID != "" {
					excs, err := s.repo.GetActiveExceptionsForWorkload(ctx, input.TenantID, w.Namespace, w.WorkloadID)
					if err == nil {
						input.Exceptions = append(input.Exceptions, excs...)
					}
				}
			}
		}

		if input.Timestamp.IsZero() {
			input.Timestamp = time.Now().UTC()
		}

		report := workflows.ExecuteAuditScan(input)

		if s.repo != nil && len(report.Findings) > 0 {
			for _, f := range report.Findings {
				if f.PolicyID == "" {
					continue
				}
				existingPol, err := s.repo.GetPolicyByID(ctx, input.TenantID, f.PolicyID)
				if err != nil || existingPol == nil {
					continue
				}
				if err := s.repo.SaveViolation(ctx, contracts.PolicyViolationRecord{
					ID:               uuid.New().String(),
					PublicID:         "viol_" + uuid.New().String()[:12],
					TenantID:         input.TenantID,
					PolicyID:         f.PolicyID,
					Source:           "AUDIT_SCAN",
					ExternalID:       fmt.Sprintf("%s:%s", f.ExternalID, f.WorkloadID),
					CorrelationID:    uuid.New().String(),
					WorkloadID:       f.WorkloadID,
					Namespace:        f.Namespace,
					Severity:         f.Severity,
					EnforcementPoint: f.Enforcement,
					Status:           contracts.ViolationStatusActive,
					Message:          f.Message,
					DetectionPair:    f.DetectionPair,
					Version:          1,
					CreatedAt:        input.Timestamp,
					UpdatedAt:        input.Timestamp,
				}); err != nil {
					observability.RecordError(observability.CurrentSpan(ctx), err)
				}
			}
		}

		return &report, nil
	}, attribute.String("tenant.id", input.TenantID), attribute.Int("workloads.count", len(input.Workloads)))
}

func (s *PolicyAsCodeService) ReconcileExpiredExceptions(ctx context.Context, tenantID string) (int, error) {
	return observability.RunTraced(ctx, "policy.service.reconcile_expired_exceptions", func(ctx context.Context) (int, error) {
		if tenantID == "" {
			return 0, errors.NewValidationError("tenantId is required")
		}
		return s.repo.ReconcileExpiredExceptions(ctx, tenantID, time.Now().UTC())
	}, attribute.String("tenant.id", tenantID))
}
