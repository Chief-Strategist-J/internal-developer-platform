package admission_test

import (
	"context"
	"testing"

	"internal-developer-platform/src/infra/clients/admission"
)

func createCompliantInput() admission.WorkloadAdmissionInput {
	return admission.WorkloadAdmissionInput{
		Namespace:         "payments-prod",
		WorkloadName:      "payment-service",
		Image:             "us-docker.pkg.dev/gov-prod/apps/payment@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:          3,
		Privileged:        false,
		PodSecurityLevel:  admission.PodSecurityLevelRestricted,
		DefaultDenyNetPol: true,
		MeshInjection:     true,
		Labels: map[string]string{
			admission.LabelRequestID:  "req-98765",
			admission.LabelOwner:      "fintech-core",
			admission.LabelCostCenter: "cc-4100",
		},
		Annotations: map[string]string{
			admission.AnnotationMeshInject: admission.MeshInjectTrue,
		},
	}
}

func TestCompliantWorkloadPasses(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if !result.Allowed {
		t.Fatalf("expected workload to be allowed, got violations: %+v", result.Violations)
	}
	if len(result.Violations) != 0 {
		t.Errorf("expected zero violations, got %d", len(result.Violations))
	}
	if result.EvaluatedCount != 6 {
		t.Errorf("expected 6 evaluated policies, got %d", result.EvaluatedCount)
	}
}

func TestPodSecurityViolationFails(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()
	input.PodSecurityLevel = admission.PodSecurityLevelBaseline

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if result.Allowed {
		t.Fatalf("expected workload with baseline pod security to be rejected")
	}

	found := false
	for _, v := range result.Violations {
		if v.PolicyID == admission.PolicyIDPodSecurityRestricted {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation %s not found in result", admission.PolicyIDPodSecurityRestricted)
	}
}

func TestMeshInjectionViolationFails(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()
	input.MeshInjection = false

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if result.Allowed {
		t.Fatalf("expected workload without mesh injection to be rejected")
	}

	found := false
	for _, v := range result.Violations {
		if v.PolicyID == admission.PolicyIDMeshInjection {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation %s not found", admission.PolicyIDMeshInjection)
	}
}

func TestDefaultDenyNetPolViolationFails(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()
	input.DefaultDenyNetPol = false

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if result.Allowed {
		t.Fatalf("expected workload without default deny netpol to be rejected")
	}

	found := false
	for _, v := range result.Violations {
		if v.PolicyID == admission.PolicyIDDefaultDenyNetPol {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation %s not found", admission.PolicyIDDefaultDenyNetPol)
	}
}

func TestMutableImageTagFails(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()
	input.Image = "us-docker.pkg.dev/gov-prod/apps/payment:v1.2.3"

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if result.Allowed {
		t.Fatalf("expected mutable image tag to be rejected")
	}

	found := false
	for _, v := range result.Violations {
		if v.PolicyID == admission.PolicyIDImmutableDigest {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation %s not found", admission.PolicyIDImmutableDigest)
	}
}

func TestMissingMandatoryLabelsFails(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()
	delete(input.Labels, admission.LabelCostCenter)

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if result.Allowed {
		t.Fatalf("expected missing cost-center label to be rejected")
	}

	found := false
	for _, v := range result.Violations {
		if v.PolicyID == admission.PolicyIDGovernanceLabels {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation %s not found", admission.PolicyIDGovernanceLabels)
	}
}

func TestPrivilegedContainerFails(t *testing.T) {
	evaluator, err := admission.NewDefaultGovernmentCELEvaluator()
	if err != nil {
		t.Fatalf("failed to create default evaluator: %v", err)
	}

	ctx := context.Background()
	input := createCompliantInput()
	input.Privileged = true

	result, err := evaluator.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error during evaluation: %v", err)
	}

	if result.Allowed {
		t.Fatalf("expected privileged container to be rejected")
	}

	found := false
	for _, v := range result.Violations {
		if v.PolicyID == admission.PolicyIDPrivilegedDisallowed {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected violation %s not found", admission.PolicyIDPrivilegedDisallowed)
	}
}

func TestInvalidRuleExpressionRejected(t *testing.T) {
	invalidRules := []admission.CELPolicyRule{
		{
			ID:         "INVALID_RULE",
			Expression: "this is invalid syntax !!!",
			Severity:   admission.SeverityBlocker,
		},
	}

	_, err := admission.NewCELEvaluator(invalidRules)
	if err == nil {
		t.Fatalf("expected error compiling invalid rule syntax, got nil")
	}
}
