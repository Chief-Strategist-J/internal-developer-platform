/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING SERVICE (V.3 role 06)

1. Purpose:
   - Stateless orchestrator implementing AutoscalingServicePort: opens a service span, strict-parses
     the input, runs the pure guardrail rules and returns the evaluation.
2. Invariants:
   - Holds no mutable state; performs no IO beyond span recording.
   - Malformed input is a validation error; a rule violation is a normal, successful Evaluation.
   - Zero inline comments below this header.
*/

package service

import (
	"context"

	"internal-developer-platform/src/features/autoscaling/internal/contracts"
	"internal-developer-platform/src/features/autoscaling/internal/domain/rules"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type AutoscalingService struct{}

func NewAutoscalingService() *AutoscalingService {
	return &AutoscalingService{}
}

func (s *AutoscalingService) EvaluatePlan(ctx context.Context, input contracts.EvaluatePlanInput) (contracts.Evaluation, error) {
	return observability.RunTraced(ctx, "autoscaling.service.evaluate_plan",
		func(ctx context.Context) (contracts.Evaluation, error) {
			if err := ctx.Err(); err != nil {
				return contracts.Evaluation{}, err
			}
			if err := contracts.EvaluateInputSchema(input); err != nil {
				return contracts.Evaluation{}, errors.NewValidationError(err.Error())
			}
			return rules.EvaluateAll(input), nil
		})
}
