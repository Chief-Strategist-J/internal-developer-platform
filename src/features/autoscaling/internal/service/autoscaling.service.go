/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING SERVICE (V.3 role 06)

1. Purpose:
   - Stateless orchestrator implementing AutoscalingServicePort: opens a service span, annotates it with the
     bounded plan context, strict-parses the input, runs the pure guardrail rules and returns the evaluation.
   - Explains itself in the trace (VIII.2): the span carries autoscaling.allowed, the violation count, the
     violated invariants and codes, and one "invariant.violated" event per violation; a rejected input
     carries autoscaling.validation.rejected and the reason. Pure rules never open spans.
2. Invariants:
   - Holds no mutable state; performs no IO beyond span recording.
   - Malformed input is a validation error; a rule violation is a normal, successful Evaluation.
   - Span attributes are bounded in cardinality and never carry personal data or workload names.
   - Zero inline comments below this header.
*/

package service

import (
	"context"
	"sort"
	"strings"

	"internal-developer-platform/src/features/autoscaling/internal/contracts"
	"internal-developer-platform/src/features/autoscaling/internal/domain/rules"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"

	"go.opentelemetry.io/otel/attribute"
)

type AutoscalingService struct{}

func NewAutoscalingService() *AutoscalingService {
	return &AutoscalingService{}
}

func (s *AutoscalingService) EvaluatePlan(ctx context.Context, input contracts.EvaluatePlanInput) (contracts.Evaluation, error) {
	return observability.RunTraced(ctx, "autoscaling.service.evaluate_plan",
		func(ctx context.Context) (contracts.Evaluation, error) {
			observability.AnnotateSpan(ctx, mapPlanToAttributes(input)...)
			if err := ctx.Err(); err != nil {
				return contracts.Evaluation{}, err
			}
			if err := contracts.EvaluateInputSchema(input); err != nil {
				observability.AnnotateSpan(ctx,
					attribute.Bool("autoscaling.validation.rejected", true),
					attribute.String("autoscaling.validation.reason", err.Error()))
				return contracts.Evaluation{}, errors.NewValidationError(err.Error())
			}
			evaluation := rules.EvaluateAll(input)
			annotateEvaluation(ctx, evaluation)
			return evaluation, nil
		})
}

func mapPlanToAttributes(input contracts.EvaluatePlanInput) []attribute.KeyValue {
	plan := input.Plan
	return []attribute.KeyValue{
		attribute.String("autoscaling.tier", plan.Tier),
		attribute.String("autoscaling.environment", string(plan.Environment)),
		attribute.String("k8s.namespace.name", plan.Workload.Namespace),
		attribute.String("k8s.workload.kind", plan.Workload.Kind),
		attribute.Int("autoscaling.min_replicas", int(plan.MinReplicas)),
		attribute.Int("autoscaling.max_replicas", int(plan.MaxReplicas)),
		attribute.Bool("autoscaling.scale_to_zero", plan.ScaleToZero),
	}
}

func mapEvaluationToSummary(evaluation contracts.Evaluation) (invariants, codes string) {
	seenInvariants := map[string]struct{}{}
	var violationCodes []string
	for _, violation := range evaluation.Violations {
		seenInvariants[violation.Invariant] = struct{}{}
		violationCodes = append(violationCodes, violation.Code)
	}
	names := make([]string, 0, len(seenInvariants))
	for name := range seenInvariants {
		names = append(names, name)
	}
	sort.Strings(names)
	sort.Strings(violationCodes)
	return strings.Join(names, ","), strings.Join(violationCodes, ",")
}

func annotateEvaluation(ctx context.Context, evaluation contracts.Evaluation) {
	invariants, violationCodes := mapEvaluationToSummary(evaluation)
	observability.AnnotateSpan(ctx,
		attribute.Bool("autoscaling.allowed", evaluation.Allowed),
		attribute.Int("autoscaling.violation.count", len(evaluation.Violations)),
		attribute.String("autoscaling.violation.invariants", invariants),
		attribute.String("autoscaling.violation.codes", violationCodes))
	for _, violation := range evaluation.Violations {
		observability.AddSpanEvent(ctx, "invariant.violated",
			attribute.String("invariant", violation.Invariant),
			attribute.String("code", violation.Code),
			attribute.String("severity", string(violation.Severity)),
			attribute.String("message", violation.Message))
	}
}
