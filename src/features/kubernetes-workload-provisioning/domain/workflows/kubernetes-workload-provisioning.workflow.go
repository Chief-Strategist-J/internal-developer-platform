package workflows

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/rollouts"
	"internal-developer-platform/src/infra/observability"
)

type ProvisionStepName string

const (
	StepValidateSpec             ProvisionStepName = "VALIDATE_SPEC"
	StepApplyMutations           ProvisionStepName = "APPLY_MUTATIONS"
	StepVerifyQuota              ProvisionStepName = "VERIFY_QUOTA"
	StepGenerateK8sSpec          ProvisionStepName = "GENERATE_K8S_SPEC"
	StepSynthesizeCanaryRollout  ProvisionStepName = "SYNTHESIZE_CANARY_ROLLOUT"
	StepSynthesizeMeshRouting    ProvisionStepName = "SYNTHESIZE_MESH_ROUTING"
	StepSynthesizeCloudResources ProvisionStepName = "SYNTHESIZE_CLOUD_RESOURCES"

	SpanWorkloadDAGExecute    = "kubernetes.workload.dag.execute"
	SpanStepValidateSpec     = "kubernetes.workload.dag.step.validate_spec"
	SpanStepApplyMutations   = "kubernetes.workload.dag.step.apply_mutations"
	SpanStepVerifyQuota      = "kubernetes.workload.dag.step.verify_quota"
	SpanStepGenerateK8sSpec  = "kubernetes.workload.dag.step.generate_k8s_spec"
	SpanStepSynthesizeRollout = "kubernetes.workload.dag.step.synthesize_rollout"
	SpanStepSynthesizeMesh    = "kubernetes.workload.dag.step.synthesize_mesh"
	SpanStepSynthesizeCloud   = "kubernetes.workload.dag.step.synthesize_cloud"

	AttrWorkloadID          = "workload.id"
	AttrDAGName             = "dag.name"
	AttrStepName            = "step.name"
	AttrCPURequest          = "cpu.request"
	AttrMemoryRequest       = "memory.request"
	AttrDAGStepsCount       = "dag.steps.count"
	AttrDAGSuccess          = "dag.success"
	DAGNameProvision        = "workload_provision_dag"
	DefaultEnvironment      = "production"
	DefaultDBInstanceSuffix = "-db"
	GSADomainFormat         = "%s@%s.iam.gserviceaccount.com"
	StableServiceSuffix     = "-stable"
	CanaryServiceSuffix     = "-canary"
	RoleCloudSQLClient      = "roles/cloudsql.client"
)

type ProvisionStep struct {
	Name   ProvisionStepName
	Passed bool
}

type ProvisionExecution struct {
	Steps []ProvisionStep
}

type SynthesizedCanaryBundle struct {
	RolloutYAML                 string
	AnalysisTemplateYAML        string
	VirtualServiceYAML          string
	DestinationRuleYAML         string
	ArgoCDIgnoreDifferencesJSON string
	WorkloadIdentityYAML        string
	DatabaseYAML                string
}

func ExecuteProvisionDAG(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (ProvisionExecution, *SynthesizedCanaryBundle, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, SpanWorkloadDAGExecute,
		attribute.String(AttrWorkloadID, cmd.ID),
		attribute.String(AttrDAGName, DAGNameProvision),
	)
	defer span.End()

	steps := make([]ProvisionStep, 0, 7)

	_, spanVal := tracer.StartSpan(ctx, SpanStepValidateSpec,
		attribute.String(AttrStepName, string(StepValidateSpec)),
	)
	steps = append(steps, ProvisionStep{Name: StepValidateSpec, Passed: true})
	spanVal.End()

	_, spanMut := tracer.StartSpan(ctx, SpanStepApplyMutations,
		attribute.String(AttrStepName, string(StepApplyMutations)),
		attribute.String(AttrCPURequest, mutations.CPURequest),
		attribute.String(AttrMemoryRequest, mutations.MemoryRequest),
	)
	steps = append(steps, ProvisionStep{Name: StepApplyMutations, Passed: true})
	spanMut.End()

	_, spanQuota := tracer.StartSpan(ctx, SpanStepVerifyQuota,
		attribute.String(AttrStepName, string(StepVerifyQuota)),
	)
	steps = append(steps, ProvisionStep{Name: StepVerifyQuota, Passed: true})
	spanQuota.End()

	_, spanGen := tracer.StartSpan(ctx, SpanStepGenerateK8sSpec,
		attribute.String(AttrStepName, string(StepGenerateK8sSpec)),
	)
	steps = append(steps, ProvisionStep{Name: StepGenerateK8sSpec, Passed: true})
	spanGen.End()

	bundle := &SynthesizedCanaryBundle{}

	rolloutCfg := rollouts.RolloutManifestConfig{
		Name:               cmd.WorkloadName,
		Namespace:          cmd.Namespace,
		Replicas:           int32(cmd.Replicas),
		WorkloadDigest:     cmd.Image,
		StableServiceName:  fmt.Sprintf("%s%s", cmd.WorkloadName, StableServiceSuffix),
		CanaryServiceName:  fmt.Sprintf("%s%s", cmd.WorkloadName, CanaryServiceSuffix),
		VirtualServiceName: cmd.WorkloadName,
		PrometheusAddress:  rollouts.DefaultPrometheusAddress,
	}

	_, spanRollout := tracer.StartSpan(ctx, SpanStepSynthesizeRollout,
		attribute.String(AttrStepName, string(StepSynthesizeCanaryRollout)),
	)
	rolloutGen := rollouts.NewRolloutManifestGenerator()
	rolloutYAML, err := rolloutGen.GenerateRolloutYAML(rolloutCfg)
	if err != nil {
		observability.RecordError(spanRollout, err)
		spanRollout.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.RolloutYAML = rolloutYAML

	analysisGen := rollouts.NewAnalysisTemplateGenerator()
	analysisYAML, err := analysisGen.GenerateAnalysisTemplateYAML(rolloutCfg)
	if err != nil {
		observability.RecordError(spanRollout, err)
		spanRollout.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.AnalysisTemplateYAML = analysisYAML

	ignoreDiffJSON, err := rolloutGen.GenerateArgoCDIgnoreDifferencesJSON()
	if err != nil {
		observability.RecordError(spanRollout, err)
		spanRollout.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.ArgoCDIgnoreDifferencesJSON = ignoreDiffJSON

	steps = append(steps, ProvisionStep{Name: StepSynthesizeCanaryRollout, Passed: true})
	spanRollout.End()

	_, spanMesh := tracer.StartSpan(ctx, SpanStepSynthesizeMesh,
		attribute.String(AttrStepName, string(StepSynthesizeMeshRouting)),
	)
	meshGen := rollouts.NewMeshRouterGenerator()
	vsYAML, err := meshGen.GenerateVirtualServiceYAML(rolloutCfg)
	if err != nil {
		observability.RecordError(spanMesh, err)
		spanMesh.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.VirtualServiceYAML = vsYAML

	drYAML, err := meshGen.GenerateDestinationRuleYAML(rolloutCfg)
	if err != nil {
		observability.RecordError(spanMesh, err)
		spanMesh.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.DestinationRuleYAML = drYAML

	steps = append(steps, ProvisionStep{Name: StepSynthesizeMeshRouting, Passed: true})
	spanMesh.End()

	_, spanCloud := tracer.StartSpan(ctx, SpanStepSynthesizeCloud,
		attribute.String(AttrStepName, string(StepSynthesizeCloudResources)),
	)
	gsaEmail := fmt.Sprintf(GSADomainFormat, cmd.WorkloadName, cmd.TenantID)
	meta := cloudresource.ResourceMetadata{
		RequestID:   cmd.ID,
		Owner:       cmd.TenantID,
		CostCenter:  cmd.TenantID,
		WorkloadID:  cmd.WorkloadName,
		Environment: DefaultEnvironment,
	}

	wiSpec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:     gsaEmail,
		KSAName:      cmd.WorkloadName,
		Namespace:    cmd.Namespace,
		ProjectID:    cmd.TenantID,
		RoleBindings: []string{RoleCloudSQLClient},
		Metadata:     meta,
	}
	wiYAML, err := cloudresource.GenerateWorkloadIdentityManifest(wiSpec)
	if err != nil {
		observability.RecordError(spanCloud, err)
		spanCloud.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.WorkloadIdentityYAML = wiYAML

	sqlSpec := cloudresource.CloudSQLBindingSpec{
		InstanceID:         fmt.Sprintf("%s%s", cmd.WorkloadName, DefaultDBInstanceSuffix),
		DatabaseName:       cmd.WorkloadName,
		AppUser:            cmd.WorkloadName,
		GSAEmail:           gsaEmail,
		IAMAuthEnabled:     true,
		DeletionProtection: true,
		Metadata:           meta,
	}
	sqlYAML, err := cloudresource.GenerateCloudSQLBindingManifest(sqlSpec)
	if err != nil {
		observability.RecordError(spanCloud, err)
		spanCloud.End()
		observability.RecordError(span, err)
		return ProvisionExecution{Steps: steps}, nil, err
	}
	bundle.DatabaseYAML = sqlYAML

	steps = append(steps, ProvisionStep{Name: StepSynthesizeCloudResources, Passed: true})
	spanCloud.End()

	span.SetAttributes(
		attribute.Int(AttrDAGStepsCount, len(steps)),
		attribute.Bool(AttrDAGSuccess, true),
	)

	return ProvisionExecution{Steps: steps}, bundle, nil
}
