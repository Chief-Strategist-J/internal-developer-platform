package rollouts

import (
	"strings"

	"internal-developer-platform/src/shared/errors"
)

const (
	RolloutAPIVersion                 = "argoproj.io/v1alpha1"
	RolloutKind                       = "Rollout"
	AnalysisTemplateKind              = "AnalysisTemplate"
	IstioAPIVersion                   = "networking.istio.io/v1alpha3"
	VirtualServiceKind                = "VirtualService"
	DestinationRuleKind               = "DestinationRule"
	MetricProviderPrometheus          = "prometheus"
	DefaultAnalysisInterval           = "30s"
	DefaultAnalysisCount              = 5
	DefaultFailureLimit               = 1
	DefaultInconclusiveLimit          = 0
	DefaultConsecutiveErrorLimit      = 1
	DefaultCanarySubset               = "canary"
	DefaultStableSubset               = "stable"
	DefaultRouteName                  = "primary"
	DefaultPrometheusAddress          = "http://prometheus-k8s.monitoring.svc.cluster.local:9090"
	DefaultMaxErrorRateDeltaCondition = "result[0] <= 0.001"
	DefaultP99LatencyCondition        = "result[0] <= 200"
	IstioMutualTLSMode                = "ISTIO_MUTUAL"

	IgnoreDiffGroupIstio   = "networking.istio.io"
	IgnoreDiffKindVS       = "VirtualService"
	IgnoreDiffRoutePointer = "/spec/http/0/route"
	IgnoreDiffGroupArgo    = "argoproj.io"
	IgnoreDiffKindRollout  = "Rollout"
	IgnoreDiffStatusPointer = "/status"

	ErrEmptyNameMsg             = "name must not be empty"
	ErrEmptyNamespaceMsg        = "namespace must not be empty"
	ErrInvalidReplicasMsg       = "replicas must be greater than zero"
	ErrEmptyWorkloadDigestMsg   = "workload digest must not be empty"
	ErrEmptyStableServiceMsg    = "stable service name must not be empty"
	ErrEmptyCanaryServiceMsg    = "canary service name must not be empty"
	ErrEmptyVirtualServiceMsg   = "virtual service name must not be empty"
	ErrEmptyPrometheusAddressMsg = "prometheus address must not be empty"
)

type CanaryStep struct {
	SetWeight     *int32 `json:"setWeight,omitempty" yaml:"setWeight,omitempty"`
	PauseDuration string `json:"pauseDuration,omitempty" yaml:"pauseDuration,omitempty"`
}

type RolloutManifestConfig struct {
	Name              string
	Namespace         string
	Replicas          int32
	WorkloadDigest    string
	StableServiceName string
	CanaryServiceName string
	VirtualServiceName string
	PrometheusAddress string
	CanarySteps       []CanaryStep
}

func (c RolloutManifestConfig) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.NewValidationError(ErrEmptyNameMsg)
	}
	if strings.TrimSpace(c.Namespace) == "" {
		return errors.NewValidationError(ErrEmptyNamespaceMsg)
	}
	if c.Replicas <= 0 {
		return errors.NewValidationError(ErrInvalidReplicasMsg)
	}
	if strings.TrimSpace(c.WorkloadDigest) == "" {
		return errors.NewValidationError(ErrEmptyWorkloadDigestMsg)
	}
	if strings.TrimSpace(c.StableServiceName) == "" {
		return errors.NewValidationError(ErrEmptyStableServiceMsg)
	}
	if strings.TrimSpace(c.CanaryServiceName) == "" {
		return errors.NewValidationError(ErrEmptyCanaryServiceMsg)
	}
	if strings.TrimSpace(c.VirtualServiceName) == "" {
		return errors.NewValidationError(ErrEmptyVirtualServiceMsg)
	}
	return nil
}

type ArgoCDIgnoreDifference struct {
	Group        string   `json:"group" yaml:"group"`
	Kind         string   `json:"kind" yaml:"kind"`
	JSONPointers []string `json:"jsonPointers" yaml:"jsonPointers"`
}

