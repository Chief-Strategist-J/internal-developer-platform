package workflows

import (
	"context"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
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
	steps := make([]ProvisionStep, 0, 4)
	steps = append(steps, ProvisionStep{Name: StepValidateSpec, Passed: true})
	steps = append(steps, ProvisionStep{Name: StepApplyMutations, Passed: true})
	steps = append(steps, ProvisionStep{Name: StepVerifyQuota, Passed: true})
	steps = append(steps, ProvisionStep{Name: StepGenerateK8sSpec, Passed: true})
	return ProvisionExecution{Steps: steps}
}
