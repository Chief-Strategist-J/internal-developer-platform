/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE DOMAIN SERVICE

1. Architecture Role:
   - Step 06 Service Orchestrator: Coordinates policy definitions, lifecycle FSM transitions, and admission evaluation.
   - Enforces OpenTelemetry child spans for all domain workflows (Tier-3 spans under "policy.service.*").
   - Interacts strictly with outbound abstract port ports.PolicyRepositoryPort.
   - Zero inline comments inside method bodies; strictly typed domain returns.
*/
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/domain/machines"
	"internal-developer-platform/src/features/policy-as-code/domain/rules"
	"internal-developer-platform/src/features/policy-as-code/domain/workflows"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/ports"
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
		return verdict, nil
	}, attribute.String("tenant.id", input.TenantID), attribute.String("workload.id", input.WorkloadID), attribute.String("namespace", input.Namespace))
}

