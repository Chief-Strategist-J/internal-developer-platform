package admission

import (
	"context"
	"fmt"

	"cel.dev/cel-go/cel"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type compiledRule struct {
	rule    CELPolicyRule
	program cel.Program
}

type CELEvaluator struct {
	env   *cel.Env
	rules []compiledRule
}

func NewCELEvaluator(rules []CELPolicyRule) (*CELEvaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable(VarPodSecurityLevel, cel.StringType),
		cel.Variable(VarMeshInjection, cel.BoolType),
		cel.Variable(VarDefaultDenyNetPol, cel.BoolType),
		cel.Variable(VarImage, cel.StringType),
		cel.Variable(VarPrivileged, cel.BoolType),
		cel.Variable(VarLabels, cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable(VarAnnotations, cel.MapType(cel.StringType, cel.StringType)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL env: %w", err)
	}

	compiledRules := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		ast, iss := env.Compile(r.Expression)
		if iss.Err() != nil {
			return nil, errors.NewValidationError(fmt.Sprintf("invalid CEL rule %s expression: %v", r.ID, iss.Err()))
		}

		prg, err := env.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("failed to compile CEL program for %s: %w", r.ID, err)
		}

		compiledRules = append(compiledRules, compiledRule{
			rule:    r,
			program: prg,
		})
	}

	return &CELEvaluator{
		env:   env,
		rules: compiledRules,
	}, nil
}

func NewDefaultGovernmentCELEvaluator() (*CELEvaluator, error) {
	return NewCELEvaluator(DefaultGovernmentPolicies)
}

func (e *CELEvaluator) Evaluate(ctx context.Context, input WorkloadAdmissionInput) (*AdmissionEvaluationResult, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, SpanAdmissionEvaluate,
		attribute.String(AttrWorkloadName, input.WorkloadName),
		attribute.String(AttrNamespace, input.Namespace),
	)
	defer span.End()

	if input.Labels == nil {
		input.Labels = make(map[string]string)
	}
	if input.Annotations == nil {
		input.Annotations = make(map[string]string)
	}

	evalCtx := map[string]any{
		VarPodSecurityLevel:  input.PodSecurityLevel,
		VarMeshInjection:     input.MeshInjection,
		VarDefaultDenyNetPol: input.DefaultDenyNetPol,
		VarImage:             input.Image,
		VarPrivileged:        input.Privileged,
		VarLabels:            input.Labels,
		VarAnnotations:       input.Annotations,
	}

	violations := make([]ViolationDetail, 0)
	for _, cr := range e.rules {
		val, _, err := cr.program.Eval(evalCtx)
		if err != nil {
			evalErr := errors.NewValidationError(fmt.Sprintf(ErrMsgEvalPrefix, cr.rule.ID, err))
			observability.RecordError(span, evalErr)
			violations = append(violations, ViolationDetail{
				PolicyID:       cr.rule.ID,
				RuleName:       cr.rule.Name,
				Severity:       cr.rule.Severity,
				FailureMessage: fmt.Sprintf(ErrMsgEvalFailure, err),
			})
			continue
		}

		passed, ok := val.Value().(bool)
		if !ok || !passed {
			violations = append(violations, ViolationDetail{
				PolicyID:       cr.rule.ID,
				RuleName:       cr.rule.Name,
				Severity:       cr.rule.Severity,
				FailureMessage: cr.rule.FailureMessage,
			})
		}
	}

	allowed := len(violations) == 0
	span.SetAttributes(
		attribute.Bool(AttrAdmissionAllowed, allowed),
		attribute.Int(AttrAdmissionViolationsCount, len(violations)),
		attribute.Int(AttrAdmissionEvaluatedCount, len(e.rules)),
	)

	return &AdmissionEvaluationResult{
		Allowed:        allowed,
		Violations:     violations,
		EvaluatedCount: len(e.rules),
	}, nil
}
