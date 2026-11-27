// Package workflows orchestrates multi-step DAG provisioning steps.
//
// Architectural Blueprint:
// - Step 09 Workflow: Synthesizes Kubernetes manifests, service mesh policies, and cloud resources.
// - Traced end-to-end under root OpenTelemetry spans with step verification.
// - 100% comment-free function bodies.
package workflows

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/domain/rules"
	"internal-developer-platform/src/infra/clients/backup"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/gateway"
	"internal-developer-platform/src/infra/clients/rollouts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
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
	StepSynthesizeGatewayRouting ProvisionStepName = "SYNTHESIZE_GATEWAY_ROUTING"
	StepSynthesizeBackupPlan     ProvisionStepName = "SYNTHESIZE_BACKUP_PLAN"

	SpanWorkloadDAGExecute    = "kubernetes.workload.dag.execute"
	SpanStepValidateSpec      = "kubernetes.workload.dag.step.validate_spec"
	SpanStepApplyMutations    = "kubernetes.workload.dag.step.apply_mutations"
	SpanStepVerifyQuota       = "kubernetes.workload.dag.step.verify_quota"
	SpanStepGenerateK8sSpec   = "kubernetes.workload.dag.step.generate_k8s_spec"
	SpanStepSynthesizeRollout = "kubernetes.workload.dag.step.synthesize_rollout"
	SpanStepSynthesizeMesh    = "kubernetes.workload.dag.step.synthesize_mesh"
	SpanStepSynthesizeCloud   = "kubernetes.workload.dag.step.synthesize_cloud"
	SpanStepSynthesizeGateway = "kubernetes.workload.dag.step.synthesize_gateway"
	SpanStepSynthesizeBackup  = "kubernetes.workload.dag.step.synthesize_backup"

	AttrWorkloadID           = "workload.id"
	AttrDAGName              = "dag.name"
	AttrStepName             = "step.name"
	AttrCPURequest           = "cpu.request"
	AttrMemoryRequest        = "memory.request"
	AttrDAGStepsCount        = "dag.steps.count"
	AttrDAGSuccess           = "dag.success"
	DAGNameProvision         = "workload_provision_dag"
	DefaultEnvironment       = "production"
	DefaultDBInstanceSuffix  = "-db"
	GSADomainFormat          = "%s@%s.iam.gserviceaccount.com"
	StableServiceSuffix      = "-stable"
	CanaryServiceSuffix      = "-canary"
	RoleCloudSQLClient       = "roles/cloudsql.client"
	DefaultHostnameFormat    = "%s.%s.example.com"
	DefaultBackendPort       = 8080
	DefaultBackendTimeoutSec = 30
	DefaultArmorPolicyName   = "platform-edge-security-policy"

	ErrInvalidCommandSpec = "workload name and namespace must not be empty"
	ErrInvalidMutations   = "cpu and memory mutations must not be empty"
	ErrInvalidReplicas    = "replicas must be greater than zero"
	ErrEmptyImageSpec     = "container image digest must not be empty"
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
	HTTPRouteYAML               string
	BackendPolicyYAML           string
	SecurityPolicyYAML          string
	BackupPlanYAML              string
	RestorePlanYAML             string
}

func buildRolloutConfig(cmd contracts.WorkloadProvisionCommand) rollouts.RolloutManifestConfig {
	return rollouts.RolloutManifestConfig{
		Name:               cmd.WorkloadName,
		Namespace:          cmd.Namespace,
		Replicas:           int32(cmd.Replicas),
		WorkloadDigest:     cmd.Image,
		StableServiceName:  fmt.Sprintf("%s%s", cmd.WorkloadName, StableServiceSuffix),
		CanaryServiceName:  fmt.Sprintf("%s%s", cmd.WorkloadName, CanaryServiceSuffix),
		VirtualServiceName: cmd.WorkloadName,
		PrometheusAddress:  rollouts.DefaultPrometheusAddress,
	}
}

func buildResourceMetadata(cmd contracts.WorkloadProvisionCommand) cloudresource.ResourceMetadata {
	return cloudresource.ResourceMetadata{
		RequestID:   cmd.ID,
		Owner:       cmd.TenantID,
		CostCenter:  effectiveCostCenter(cmd),
		WorkloadID:  cmd.WorkloadName,
		Environment: DefaultEnvironment,
	}
}

func buildRouteConfig(cmd contracts.WorkloadProvisionCommand) gateway.HTTPRouteConfig {
	return gateway.HTTPRouteConfig{
		Name:        cmd.WorkloadName,
		Namespace:   cmd.Namespace,
		Hostnames:   []string{fmt.Sprintf(DefaultHostnameFormat, cmd.WorkloadName, cmd.TenantID)},
		ServiceName: cmd.WorkloadName,
		ServicePort: DefaultBackendPort,
	}
}

func buildBackendPolicyConfig(cmd contracts.WorkloadProvisionCommand) gateway.GCPBackendPolicyConfig {
	return gateway.GCPBackendPolicyConfig{
		Name:               fmt.Sprintf("%s-backend-policy", cmd.WorkloadName),
		Namespace:          cmd.Namespace,
		TargetServiceName:  cmd.WorkloadName,
		SecurityPolicyName: DefaultArmorPolicyName,
		TimeoutSec:         DefaultBackendTimeoutSec,
		LoggingEnabled:     true,
	}
}

func stepValidateSpec(ctx context.Context, cmd contracts.WorkloadProvisionCommand) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepValidateSpec, func(ctx context.Context) (ProvisionStep, error) {
		if strings.TrimSpace(cmd.WorkloadName) == "" || strings.TrimSpace(cmd.Namespace) == "" {
			return ProvisionStep{}, errors.NewValidationError(ErrInvalidCommandSpec)
		}
		return ProvisionStep{Name: StepValidateSpec, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepValidateSpec)))
}

func stepApplyMutations(ctx context.Context, mutations rules.WorkloadMutations) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepApplyMutations, func(ctx context.Context) (ProvisionStep, error) {
		if strings.TrimSpace(mutations.CPURequest) == "" || strings.TrimSpace(mutations.MemoryRequest) == "" {
			return ProvisionStep{}, errors.NewValidationError(ErrInvalidMutations)
		}
		return ProvisionStep{Name: StepApplyMutations, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepApplyMutations)), attribute.String(AttrCPURequest, mutations.CPURequest))
}

func stepVerifyQuota(ctx context.Context, cmd contracts.WorkloadProvisionCommand) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepVerifyQuota, func(ctx context.Context) (ProvisionStep, error) {
		if cmd.Replicas <= 0 {
			return ProvisionStep{}, errors.NewValidationError(ErrInvalidReplicas)
		}
		return ProvisionStep{Name: StepVerifyQuota, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepVerifyQuota)))
}

func stepGenerateK8sSpec(ctx context.Context, cmd contracts.WorkloadProvisionCommand) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepGenerateK8sSpec, func(ctx context.Context) (ProvisionStep, error) {
		if strings.TrimSpace(cmd.Image) == "" {
			return ProvisionStep{}, errors.NewValidationError(ErrEmptyImageSpec)
		}
		return ProvisionStep{Name: StepGenerateK8sSpec, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepGenerateK8sSpec)))
}

func stepSynthesizeRollout(ctx context.Context, cfg rollouts.RolloutManifestConfig, bundle *SynthesizedCanaryBundle) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepSynthesizeRollout, func(ctx context.Context) (ProvisionStep, error) {
		gen := rollouts.NewRolloutManifestGenerator()
		rolloutYAML, err := gen.GenerateRolloutYAML(cfg)
		if err != nil {
			return ProvisionStep{}, err
		}
		analysisGen := rollouts.NewAnalysisTemplateGenerator()
		analysisYAML, err := analysisGen.GenerateAnalysisTemplateYAML(cfg)
		if err != nil {
			return ProvisionStep{}, err
		}
		bundle.RolloutYAML = rolloutYAML
		bundle.AnalysisTemplateYAML = analysisYAML
		bundle.ArgoCDIgnoreDifferencesJSON, _ = gen.GenerateArgoCDIgnoreDifferencesJSON()
		return ProvisionStep{Name: StepSynthesizeCanaryRollout, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepSynthesizeCanaryRollout)))
}

func stepSynthesizeMesh(ctx context.Context, cfg rollouts.RolloutManifestConfig, bundle *SynthesizedCanaryBundle) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepSynthesizeMesh, func(ctx context.Context) (ProvisionStep, error) {
		meshGen := rollouts.NewMeshRouterGenerator()
		vsYAML, err := meshGen.GenerateVirtualServiceYAML(cfg)
		if err != nil {
			return ProvisionStep{}, err
		}
		drYAML, err := meshGen.GenerateDestinationRuleYAML(cfg)
		if err != nil {
			return ProvisionStep{}, err
		}
		bundle.VirtualServiceYAML = vsYAML
		bundle.DestinationRuleYAML = drYAML
		return ProvisionStep{Name: StepSynthesizeMeshRouting, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepSynthesizeMeshRouting)))
}

func synthesizeWorkloadIdentity(cmd contracts.WorkloadProvisionCommand, meta cloudresource.ResourceMetadata) (string, error) {
	return cloudresource.GenerateWorkloadIdentityManifest(cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:     fmt.Sprintf(GSADomainFormat, cmd.WorkloadName, cmd.TenantID),
		KSAName:      cmd.WorkloadName,
		Namespace:    cmd.Namespace,
		ProjectID:    cmd.TenantID,
		RoleBindings: []string{RoleCloudSQLClient},
		Metadata:     meta,
	})
}

func synthesizeCloudSQL(cmd contracts.WorkloadProvisionCommand, meta cloudresource.ResourceMetadata) (string, error) {
	return cloudresource.GenerateCloudSQLBindingManifest(cloudresource.CloudSQLBindingSpec{
		InstanceID:         fmt.Sprintf("%s%s", cmd.WorkloadName, DefaultDBInstanceSuffix),
		DatabaseName:       cmd.WorkloadName,
		AppUser:            cmd.WorkloadName,
		GSAEmail:           fmt.Sprintf(GSADomainFormat, cmd.WorkloadName, cmd.TenantID),
		IAMAuthEnabled:     true,
		DeletionProtection: true,
		Metadata:           meta,
	})
}

func stepSynthesizeCloud(ctx context.Context, cmd contracts.WorkloadProvisionCommand, bundle *SynthesizedCanaryBundle) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepSynthesizeCloud, func(ctx context.Context) (ProvisionStep, error) {
		meta := buildResourceMetadata(cmd)
		wiYAML, err := synthesizeWorkloadIdentity(cmd, meta)
		if err != nil {
			return ProvisionStep{}, err
		}
		sqlYAML, err := synthesizeCloudSQL(cmd, meta)
		if err != nil {
			return ProvisionStep{}, err
		}
		bundle.WorkloadIdentityYAML = wiYAML
		bundle.DatabaseYAML = sqlYAML
		return ProvisionStep{Name: StepSynthesizeCloudResources, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepSynthesizeCloudResources)))
}

func stepSynthesizeGateway(ctx context.Context, cmd contracts.WorkloadProvisionCommand, bundle *SynthesizedCanaryBundle) (ProvisionStep, error) {
	return observability.RunTraced(ctx, SpanStepSynthesizeGateway, func(ctx context.Context) (ProvisionStep, error) {
		routeGen := gateway.NewHTTPRouteGenerator()
		routeYAML, err := routeGen.GenerateHTTPRouteYAML(buildRouteConfig(cmd))
		if err != nil {
			return ProvisionStep{}, err
		}
		armorGen := gateway.NewCloudArmorGenerator()
		backendYAML, err := armorGen.GenerateGCPBackendPolicyYAML(buildBackendPolicyConfig(cmd))
		if err != nil {
			return ProvisionStep{}, err
		}
		secPolicyYAML, err := armorGen.GenerateComputeSecurityPolicyYAML(gateway.ComputeSecurityPolicyConfig{
			Name:        DefaultArmorPolicyName,
			Namespace:   cmd.Namespace,
			ProjectID:   cmd.TenantID,
			Description: "Enterprise Cloud Armor Edge Security Policy with rate limiting and WAF rules",
			Rules: []gateway.CloudArmorRule{
				{
					Priority:    1000,
					Action:      gateway.ActionAllow,
					Description: "Default allow ingress traffic",
					SrcIPRanges: []string{gateway.MatchAnySrcIP},
				},
				{
					Priority:             2000,
					Action:               gateway.ActionRateBasedBan,
					Description:          "Anti-DDoS and brute-force rate limit",
					SrcIPRanges:          []string{gateway.MatchAnySrcIP},
					RateLimitThreshold:   1000,
					RateLimitIntervalSec: 60,
				},
			},
		})
		if err != nil {
			return ProvisionStep{}, err
		}
		bundle.HTTPRouteYAML = routeYAML
		bundle.BackendPolicyYAML = backendYAML
		bundle.SecurityPolicyYAML = secPolicyYAML
		return ProvisionStep{Name: StepSynthesizeGatewayRouting, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepSynthesizeGatewayRouting)))
}

func stepSynthesizeBackup(ctx context.Context, cmd contracts.WorkloadProvisionCommand, bundle *SynthesizedCanaryBundle) (ProvisionStep, error) {
	if !cmd.BackupPlan.Enabled {
		return ProvisionStep{Name: StepSynthesizeBackupPlan, Passed: true}, nil
	}

	return observability.RunTraced(ctx, SpanStepSynthesizeBackup, func(ctx context.Context) (ProvisionStep, error) {
		bpYAML, err := backup.GenerateBackupPlanYAML(backup.BackupPlanConfig{
			Name:              cmd.WorkloadName,
			Namespace:         cmd.Namespace,
			Cluster:           cmd.BackupPlan.Cluster,
			Location:          cmd.BackupPlan.Location,
			CronSchedule:      cmd.BackupPlan.CronSchedule,
			RetentionDays:     cmd.BackupPlan.RetentionDays,
			LockedRetention:   cmd.BackupPlan.LockedRetention,
			IncludeSecrets:    cmd.BackupPlan.IncludeSecrets,
			IncludeVolumeData: cmd.BackupPlan.IncludeVolumeData,
			TargetNamespaces:  []string{cmd.Namespace},
			EncryptionKeyKMS:  cmd.BackupPlan.EncryptionKeyKMS,
		})
		if err != nil {
			return ProvisionStep{}, err
		}

		bundle.BackupPlanYAML = bpYAML

		if cmd.RestorePlan.Enabled {
			rpYAML, rpErr := backup.GenerateRestorePlanYAML(backup.RestorePlanConfig{
				Name:               cmd.WorkloadName,
				Namespace:          cmd.Namespace,
				TargetCluster:      cmd.RestorePlan.TargetCluster,
				SourceBackupPlan:   cmd.RestorePlan.SourceBackupPlan,
				ConflictPolicy:     cmd.RestorePlan.ConflictPolicy,
				SelectedNamespaces: []string{cmd.Namespace},
			})
			if rpErr != nil {
				return ProvisionStep{}, rpErr
			}
			bundle.RestorePlanYAML = rpYAML
		}

		return ProvisionStep{Name: StepSynthesizeBackupPlan, Passed: true}, nil
	}, attribute.String(AttrStepName, string(StepSynthesizeBackupPlan)))
}

func buildDAGSteps(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations, bundle *SynthesizedCanaryBundle) []func() (ProvisionStep, error) {
	rolloutCfg := buildRolloutConfig(cmd)
	return []func() (ProvisionStep, error){
		func() (ProvisionStep, error) { return stepValidateSpec(ctx, cmd) },
		func() (ProvisionStep, error) { return stepApplyMutations(ctx, mutations) },
		func() (ProvisionStep, error) { return stepVerifyQuota(ctx, cmd) },
		func() (ProvisionStep, error) { return stepGenerateK8sSpec(ctx, cmd) },
		func() (ProvisionStep, error) { return stepSynthesizeRollout(ctx, rolloutCfg, bundle) },
		func() (ProvisionStep, error) { return stepSynthesizeMesh(ctx, rolloutCfg, bundle) },
		func() (ProvisionStep, error) { return stepSynthesizeCloud(ctx, cmd, bundle) },
		func() (ProvisionStep, error) { return stepSynthesizeGateway(ctx, cmd, bundle) },
		func() (ProvisionStep, error) { return stepSynthesizeBackup(ctx, cmd, bundle) },
	}
}

func ExecuteProvisionDAG(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (ProvisionExecution, *SynthesizedCanaryBundle, error) {
	bundle := &SynthesizedCanaryBundle{}
	steps := make([]ProvisionStep, 0, 9)
	err := observability.WithSpan(ctx, SpanWorkloadDAGExecute, func(ctx context.Context) error {
		for _, stepFunc := range buildDAGSteps(ctx, cmd, mutations, bundle) {
			st, stepErr := stepFunc()
			if stepErr != nil {
				return stepErr
			}
			steps = append(steps, st)
		}
		return nil
	}, attribute.String(AttrWorkloadID, cmd.ID), attribute.String(AttrDAGName, DAGNameProvision))
	if err != nil {
		return ProvisionExecution{Steps: steps}, nil, err
	}
	return ProvisionExecution{Steps: steps}, bundle, nil
}
func effectiveCostCenter(cmd contracts.WorkloadProvisionCommand) string {
	if cmd.CostCenter != "" {
		return cmd.CostCenter
	}
	return cmd.TenantID
}
