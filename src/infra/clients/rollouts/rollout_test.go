package rollouts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TestWorkloadName      = "payment-service"
	TestNamespaceName     = "production"
	TestReplicas          = int32(5)
	TestDigest            = "gcr.io/corp-prod/payment@sha256:4f355bd3a39e728ec65147be15e1fa6dc9e20dd4a218f02934ffea210087799e"
	TestStableSvc         = "payment-service-stable"
	TestCanarySvc         = "payment-service-canary"
	TestVirtualSvc        = "payment-service"
	TestPrometheusServer  = "http://prometheus.monitoring.svc:9090"
)

func validTestConfig() RolloutManifestConfig {
	return RolloutManifestConfig{
		Name:               TestWorkloadName,
		Namespace:          TestNamespaceName,
		Replicas:           TestReplicas,
		WorkloadDigest:     TestDigest,
		StableServiceName:  TestStableSvc,
		CanaryServiceName:  TestCanarySvc,
		VirtualServiceName: TestVirtualSvc,
		PrometheusAddress:  TestPrometheusServer,
	}
}

func TestRolloutConfigValidation(t *testing.T) {
	cfg := validTestConfig()
	require.NoError(t, cfg.Validate())

	emptyName := cfg
	emptyName.Name = ""
	assert.Error(t, emptyName.Validate())

	emptyNamespace := cfg
	emptyNamespace.Namespace = ""
	assert.Error(t, emptyNamespace.Validate())

	zeroReplicas := cfg
	zeroReplicas.Replicas = 0
	assert.Error(t, zeroReplicas.Validate())

	emptyDigest := cfg
	emptyDigest.WorkloadDigest = ""
	assert.Error(t, emptyDigest.Validate())

	emptyStableSvc := cfg
	emptyStableSvc.StableServiceName = ""
	assert.Error(t, emptyStableSvc.Validate())

	emptyCanarySvc := cfg
	emptyCanarySvc.CanaryServiceName = ""
	assert.Error(t, emptyCanarySvc.Validate())

	emptyVirtualSvc := cfg
	emptyVirtualSvc.VirtualServiceName = ""
	assert.Error(t, emptyVirtualSvc.Validate())
}

func TestRolloutManifestGenerator(t *testing.T) {
	cfg := validTestConfig()
	gen := NewRolloutManifestGenerator()

	rollout, err := gen.GenerateRollout(cfg)
	require.NoError(t, err)
	assert.Equal(t, RolloutAPIVersion, rollout["apiVersion"])
	assert.Equal(t, RolloutKind, rollout["kind"])

	spec, ok := rollout["spec"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, TestReplicas, spec["replicas"])

	strategy, ok := spec["strategy"].(map[string]interface{})
	require.True(t, ok)
	canary, ok := strategy["canary"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, TestStableSvc, canary["stableService"])
	assert.Equal(t, TestCanarySvc, canary["canaryService"])

	steps, ok := canary["steps"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, steps, 4)
	assert.Equal(t, int32(10), steps[0]["setWeight"])
	assert.Equal(t, int32(25), steps[1]["setWeight"])
	assert.Equal(t, int32(50), steps[2]["setWeight"])
	assert.Equal(t, int32(100), steps[3]["setWeight"])

	yamlStr, err := gen.GenerateRolloutYAML(cfg)
	require.NoError(t, err)
	assert.Contains(t, yamlStr, "apiVersion: argoproj.io/v1alpha1")
	assert.Contains(t, yamlStr, "kind: Rollout")
	assert.Contains(t, yamlStr, TestDigest)

	diffs := gen.GenerateArgoCDIgnoreDifferences()
	require.Len(t, diffs, 2)
	assert.Equal(t, IgnoreDiffGroupIstio, diffs[0].Group)
	assert.Equal(t, IgnoreDiffKindVS, diffs[0].Kind)
	assert.Contains(t, diffs[0].JSONPointers, IgnoreDiffRoutePointer)
	assert.Equal(t, IgnoreDiffGroupArgo, diffs[1].Group)
	assert.Equal(t, IgnoreDiffKindRollout, diffs[1].Kind)
	assert.Contains(t, diffs[1].JSONPointers, IgnoreDiffStatusPointer)

	jsonStr, err := gen.GenerateArgoCDIgnoreDifferencesJSON()
	require.NoError(t, err)
	assert.Contains(t, jsonStr, IgnoreDiffRoutePointer)
	assert.Contains(t, jsonStr, IgnoreDiffStatusPointer)
}

func TestAnalysisTemplateGenerator(t *testing.T) {
	cfg := validTestConfig()
	gen := NewAnalysisTemplateGenerator()

	template, err := gen.GenerateAnalysisTemplate(cfg)
	require.NoError(t, err)
	assert.Equal(t, RolloutAPIVersion, template["apiVersion"])
	assert.Equal(t, AnalysisTemplateKind, template["kind"])

	spec, ok := template["spec"].(map[string]interface{})
	require.True(t, ok)
	metrics, ok := spec["metrics"].([]interface{})
	require.True(t, ok)
	require.Len(t, metrics, 2)

	m1, ok := metrics[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, MetricCanaryErrorRateDelta, m1["name"])
	assert.Equal(t, DefaultFailureLimit, m1["failureLimit"])
	assert.Equal(t, DefaultConsecutiveErrorLimit, m1["consecutiveErrorLimit"])

	m2, ok := metrics[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, MetricCanaryP99Latency, m2["name"])
	assert.Equal(t, DefaultFailureLimit, m2["failureLimit"])
	assert.Equal(t, DefaultConsecutiveErrorLimit, m2["consecutiveErrorLimit"])

	yamlStr, err := gen.GenerateAnalysisTemplateYAML(cfg)
	require.NoError(t, err)
	assert.Contains(t, yamlStr, MetricCanaryErrorRateDelta)
	assert.Contains(t, yamlStr, MetricCanaryP99Latency)
	assert.Contains(t, yamlStr, DefaultMaxErrorRateDeltaCondition)
	assert.Contains(t, yamlStr, DefaultP99LatencyCondition)
}

func TestMeshRouterGenerator(t *testing.T) {
	cfg := validTestConfig()
	gen := NewMeshRouterGenerator()

	vs, err := gen.GenerateVirtualService(cfg)
	require.NoError(t, err)
	assert.Equal(t, IstioAPIVersion, vs["apiVersion"])
	assert.Equal(t, VirtualServiceKind, vs["kind"])

	dr, err := gen.GenerateDestinationRule(cfg)
	require.NoError(t, err)
	assert.Equal(t, IstioAPIVersion, dr["apiVersion"])
	assert.Equal(t, DestinationRuleKind, dr["kind"])

	spec, ok := dr["spec"].(map[string]interface{})
	require.True(t, ok)
	trafficPolicy, ok := spec["trafficPolicy"].(map[string]interface{})
	require.True(t, ok)
	tls, ok := trafficPolicy["tls"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, IstioMutualTLSMode, tls["mode"])

	vsYaml, err := gen.GenerateVirtualServiceYAML(cfg)
	require.NoError(t, err)
	assert.Contains(t, vsYaml, "kind: VirtualService")

	drYaml, err := gen.GenerateDestinationRuleYAML(cfg)
	require.NoError(t, err)
	assert.Contains(t, drYaml, "kind: DestinationRule")
	assert.Contains(t, drYaml, "mode: ISTIO_MUTUAL")
}

