package rollouts

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"internal-developer-platform/src/shared/errors"
)

const (
	DefaultStep1Weight = 10
	DefaultStep1Pause  = "5m"
	DefaultStep2Weight = 25
	DefaultStep2Pause  = "10m"
	DefaultStep3Weight = 50
	DefaultStep3Pause  = "15m"
	DefaultStep4Weight = 100
)

type RolloutManifestGenerator struct{}

func NewRolloutManifestGenerator() *RolloutManifestGenerator {
	return &RolloutManifestGenerator{}
}

func (g *RolloutManifestGenerator) GenerateRollout(config RolloutManifestConfig) (map[string]interface{}, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	steps := config.CanarySteps
	if len(steps) == 0 {
		steps = g.defaultCanarySteps()
	}

	manifestSteps := make([]map[string]interface{}, 0, len(steps))
	for _, step := range steps {
		stepMap := make(map[string]interface{})
		if step.SetWeight != nil {
			stepMap["setWeight"] = *step.SetWeight
		}
		if step.PauseDuration != "" {
			stepMap["pause"] = map[string]interface{}{
				"duration": step.PauseDuration,
			}
		}
		manifestSteps = append(manifestSteps, stepMap)
	}

	analysisTemplateName := fmt.Sprintf("%s-metrics-gate", config.Name)

	rollout := map[string]interface{}{
		"apiVersion": RolloutAPIVersion,
		"kind":       RolloutKind,
		"metadata": map[string]interface{}{
			"name":      config.Name,
			"namespace": config.Namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name":       config.Name,
				"app.kubernetes.io/managed-by": "argo-rollouts",
			},
		},
		"spec": map[string]interface{}{
			"replicas": config.Replicas,
			"strategy": map[string]interface{}{
				"canary": map[string]interface{}{
					"stableService": config.StableServiceName,
					"canaryService": config.CanaryServiceName,
					"trafficRouting": map[string]interface{}{
						"istio": map[string]interface{}{
							"virtualService": map[string]interface{}{
								"name":   config.VirtualServiceName,
								"routes": []interface{}{DefaultRouteName},
							},
							"destinationRule": map[string]interface{}{
								"name":             config.Name,
								"canarySubsetName": DefaultCanarySubset,
								"stableSubsetName": DefaultStableSubset,
							},
						},
					},
					"analysis": map[string]interface{}{
						"templates": []interface{}{
							map[string]interface{}{
								"templateName": analysisTemplateName,
							},
						},
					},
					"steps": manifestSteps,
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app.kubernetes.io/name": config.Name,
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  config.Name,
							"image": config.WorkloadDigest,
						},
					},
				},
			},
		},
	}

	return rollout, nil
}

func (g *RolloutManifestGenerator) GenerateRolloutYAML(config RolloutManifestConfig) (string, error) {
	manifest, err := g.GenerateRollout(config)
	if err != nil {
		return "", err
	}
	bytes, err := yaml.Marshal(manifest)
	if err != nil {
		return "", errors.NewValidationError(err.Error())
	}
	return strings.TrimSpace(string(bytes)), nil
}

func (g *RolloutManifestGenerator) GenerateArgoCDIgnoreDifferences() []ArgoCDIgnoreDifference {
	return []ArgoCDIgnoreDifference{
		{
			Group:        IgnoreDiffGroupIstio,
			Kind:         IgnoreDiffKindVS,
			JSONPointers: []string{IgnoreDiffRoutePointer},
		},
		{
			Group:        IgnoreDiffGroupArgo,
			Kind:         IgnoreDiffKindRollout,
			JSONPointers: []string{IgnoreDiffStatusPointer},
		},
	}
}

func (g *RolloutManifestGenerator) GenerateArgoCDIgnoreDifferencesJSON() (string, error) {
	diffs := g.GenerateArgoCDIgnoreDifferences()
	bytes, err := json.Marshal(diffs)
	if err != nil {
		return "", errors.NewValidationError(err.Error())
	}
	return string(bytes), nil
}

func (g *RolloutManifestGenerator) defaultCanarySteps() []CanaryStep {
	w1 := int32(DefaultStep1Weight)
	w2 := int32(DefaultStep2Weight)
	w3 := int32(DefaultStep3Weight)
	w4 := int32(DefaultStep4Weight)

	return []CanaryStep{
		{SetWeight: &w1, PauseDuration: DefaultStep1Pause},
		{SetWeight: &w2, PauseDuration: DefaultStep2Pause},
		{SetWeight: &w3, PauseDuration: DefaultStep3Pause},
		{SetWeight: &w4},
	}
}
