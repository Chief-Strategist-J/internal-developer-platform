package workflows

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/infra/observability"
)

const (
	SpanWorkloadDecommissionDAG = "kubernetes.workload.dag.decommission"
	AttrDecommissionStep       = "decommission.step"
)

type DecommissionDAGStep struct {
	Step   contracts.DecommissionExecutionStep
	Passed bool
}

func ExecuteDecommissionDAG(ctx context.Context, cmd contracts.WorkloadDecommissionCommand) ([]contracts.DecommissionExecutionStep, error) {
	executed := make([]contracts.DecommissionExecutionStep, 0, 7)
	err := observability.WithSpan(ctx, SpanWorkloadDecommissionDAG, func(ctx context.Context) error {
		steps := []contracts.DecommissionExecutionStep{
			contracts.StepDecommissionDrainTraffic,
			contracts.StepDecommissionRemoveWorkload,
			contracts.StepDecommissionRemoveBindings,
			contracts.StepDecommissionReleaseStorage,
			contracts.StepDecommissionDeprovisionIdentity,
			contracts.StepDecommissionRemoveDNS,
			contracts.StepDecommissionRecordAudit,
		}

		for _, step := range steps {
			stepErr := observability.WithSpan(ctx, fmt.Sprintf("kubernetes.workload.decommission.step.%s", step), func(ctx context.Context) error {
				executed = append(executed, step)
				return nil
			}, attribute.String(AttrDecommissionStep, string(step)))
			if stepErr != nil {
				return stepErr
			}
		}
		return nil
	}, attribute.String(AttrWorkloadID, cmd.WorkloadName), attribute.String(AttrDAGName, "workload_decommission_dag"))

	if err != nil {
		return executed, err
	}
	return executed, nil
}
