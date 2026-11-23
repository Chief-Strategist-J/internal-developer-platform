package rollouts

import (
	"strings"

	"gopkg.in/yaml.v3"

	"internal-developer-platform/src/shared/errors"
)

type MeshRouterGenerator struct{}

func NewMeshRouterGenerator() *MeshRouterGenerator {
	return &MeshRouterGenerator{}
}

func (g *MeshRouterGenerator) GenerateVirtualService(config RolloutManifestConfig) (map[string]interface{}, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	vs := map[string]interface{}{
		"apiVersion": IstioAPIVersion,
		"kind":       VirtualServiceKind,
		"metadata": map[string]interface{}{
			"name":      config.VirtualServiceName,
			"namespace": config.Namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name": config.Name,
			},
		},
		"spec": map[string]interface{}{
			"hosts": []interface{}{
				config.VirtualServiceName,
			},
			"http": []interface{}{
				map[string]interface{}{
					"name": DefaultRouteName,
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host":   config.StableServiceName,
								"subset": DefaultStableSubset,
							},
							"weight": 100,
						},
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host":   config.CanaryServiceName,
								"subset": DefaultCanarySubset,
							},
							"weight": 0,
						},
					},
				},
			},
		},
	}

	return vs, nil
}

func (g *MeshRouterGenerator) GenerateDestinationRule(config RolloutManifestConfig) (map[string]interface{}, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	dr := map[string]interface{}{
		"apiVersion": IstioAPIVersion,
		"kind":       DestinationRuleKind,
		"metadata": map[string]interface{}{
			"name":      config.Name,
			"namespace": config.Namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name": config.Name,
			},
		},
		"spec": map[string]interface{}{
			"host": config.VirtualServiceName,
			"trafficPolicy": map[string]interface{}{
				"tls": map[string]interface{}{
					"mode": IstioMutualTLSMode,
				},
			},
			"subsets": []interface{}{
				map[string]interface{}{
					"name": DefaultStableSubset,
					"labels": map[string]interface{}{
						"role": DefaultStableSubset,
					},
				},
				map[string]interface{}{
					"name": DefaultCanarySubset,
					"labels": map[string]interface{}{
						"role": DefaultCanarySubset,
					},
				},
			},
		},
	}

	return dr, nil
}

func (g *MeshRouterGenerator) GenerateVirtualServiceYAML(config RolloutManifestConfig) (string, error) {
	manifest, err := g.GenerateVirtualService(config)
	if err != nil {
		return "", err
	}
	bytes, err := yaml.Marshal(manifest)
	if err != nil {
		return "", errors.NewValidationError(err.Error())
	}
	return strings.TrimSpace(string(bytes)), nil
}

func (g *MeshRouterGenerator) GenerateDestinationRuleYAML(config RolloutManifestConfig) (string, error) {
	manifest, err := g.GenerateDestinationRule(config)
	if err != nil {
		return "", err
	}
	bytes, err := yaml.Marshal(manifest)
	if err != nil {
		return "", errors.NewValidationError(err.Error())
	}
	return strings.TrimSpace(string(bytes)), nil
}
