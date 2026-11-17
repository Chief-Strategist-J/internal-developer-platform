package workflows

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/infra/observability"
)

type ProvisionStepName string

const (
	StepValidateSpec    ProvisionStepName = "VALIDATE_SPEC"
	StepApplyMutations  ProvisionStepName = "APPLY_MUTATIONS"
	StepVerifyQuota     ProvisionStepName = "VERIFY_QUOTA"
	StepGenerateK8sSpec ProvisionStepName = "GENERATE_K8S_SPEC"
)

type ProvisionStep struct {
	Name   ProvisionStepName
	Passed bool
}

type ProvisionExecution struct {
	Steps []ProvisionStep
}

func ExecuteProvisionDAG(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) ProvisionExecution {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "kubernetes.workload.dag.execute",
		attribute.String("workload.id", cmd.ID),
		attribute.String("dag.name", "workload_provision_dag"),
	)
	defer span.End()

	steps := make([]ProvisionStep, 0, 4)

	_, spanVal := tracer.StartSpan(ctx, "kubernetes.workload.dag.step.validate_spec",
		attribute.String("step.name", string(StepValidateSpec)),
	)
	steps = append(steps, ProvisionStep{Name: StepValidateSpec, Passed: true})
	spanVal.End()

	_, spanMut := tracer.StartSpan(ctx, "kubernetes.workload.dag.step.apply_mutations",
		attribute.String("step.name", string(StepApplyMutations)),
		attribute.String("cpu.request", mutations.CPURequest),
		attribute.String("memory.request", mutations.MemoryRequest),
	)
	steps = append(steps, ProvisionStep{Name: StepApplyMutations, Passed: true})
	spanMut.End()

	_, spanQuota := tracer.StartSpan(ctx, "kubernetes.workload.dag.step.verify_quota",
		attribute.String("step.name", string(StepVerifyQuota)),
	)
	steps = append(steps, ProvisionStep{Name: StepVerifyQuota, Passed: true})
	spanQuota.End()

	_, spanGen := tracer.StartSpan(ctx, "kubernetes.workload.dag.step.generate_k8s_spec",
		attribute.String("step.name", string(StepGenerateK8sSpec)),
	)
	steps = append(steps, ProvisionStep{Name: StepGenerateK8sSpec, Passed: true})
	spanGen.End()

	span.SetAttributes(
		attribute.Int("dag.steps.count", len(steps)),
		attribute.Bool("dag.success", true),
	)

	return ProvisionExecution{Steps: steps}
}
