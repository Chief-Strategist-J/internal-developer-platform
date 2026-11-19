package rollouts

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"internal-developer-platform/src/shared/errors"
)

const (
	MetricCanaryErrorRateDelta = "canary-error-rate-delta"
	MetricCanaryP99Latency     = "canary-p99-latency"

	PromQLErrorRateDeltaFormat = `(sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-canary",response_code=~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-canary"}[1m]))) - (sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-stable",response_code=~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-stable"}[1m])))`
	PromQLP99LatencyFormat     = `histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{reporter="destination",destination_workload="%s-canary"}[1m])) by (le))`
)

type AnalysisTemplateGenerator struct{}

func NewAnalysisTemplateGenerator() *AnalysisTemplateGenerator {
	return &AnalysisTemplateGenerator{}
}

func (g *AnalysisTemplateGenerator) GenerateAnalysisTemplate(config RolloutManifestConfig) (map[string]interface{}, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	prometheusAddr := config.PrometheusAddress
	if prometheusAddr == "" {
		prometheusAddr = DefaultPrometheusAddress
	}

	templateName := fmt.Sprintf("%s-metrics-gate", config.Name)
	errorRateQuery := fmt.Sprintf(PromQLErrorRateDeltaFormat, config.Name, config.Name, config.Name, config.Name)
	latencyQuery := fmt.Sprintf(PromQLP99LatencyFormat, config.Name)

	template := map[string]interface{}{
		"apiVersion": RolloutAPIVersion,
		"kind":       AnalysisTemplateKind,
		"metadata": map[string]interface{}{
			"name":      templateName,
			"namespace": config.Namespace,
			"labels": map[string]interface{}{
				"app.kubernetes.io/name":       config.Name,
				"app.kubernetes.io/managed-by": "argo-rollouts",
			},
		},
		"spec": map[string]interface{}{
			"metrics": []interface{}{
				map[string]interface{}{
					"name":                  MetricCanaryErrorRateDelta,
					"interval":              DefaultAnalysisInterval,
					"count":                 DefaultAnalysisCount,
					"failureLimit":          DefaultFailureLimit,
					"inconclusiveLimit":     DefaultInconclusiveLimit,
					"consecutiveErrorLimit": DefaultConsecutiveErrorLimit,
					"successCondition":      DefaultMaxErrorRateDeltaCondition,
					"provider": map[string]interface{}{
						MetricProviderPrometheus: map[string]interface{}{
							"address": prometheusAddr,
							"query":   errorRateQuery,
						},
					},
				},
				map[string]interface{}{
					"name":                  MetricCanaryP99Latency,
					"interval":              DefaultAnalysisInterval,
					"count":                 DefaultAnalysisCount,
					"failureLimit":          DefaultFailureLimit,
					"inconclusiveLimit":     DefaultInconclusiveLimit,
					"consecutiveErrorLimit": DefaultConsecutiveErrorLimit,
					"successCondition":      DefaultP99LatencyCondition,
					"provider": map[string]interface{}{
						MetricProviderPrometheus: map[string]interface{}{
							"address": prometheusAddr,
							"query":   latencyQuery,
						},
					},
				},
			},
		},
	}

	return template, nil
}

func (g *AnalysisTemplateGenerator) GenerateAnalysisTemplateYAML(config RolloutManifestConfig) (string, error) {
	manifest, err := g.GenerateAnalysisTemplate(config)
	if err != nil {
		return "", err
	}
	bytes, err := yaml.Marshal(manifest)
	if err != nil {
		return "", errors.NewValidationError(err.Error())
	}
	return strings.TrimSpace(string(bytes)), nil
}

