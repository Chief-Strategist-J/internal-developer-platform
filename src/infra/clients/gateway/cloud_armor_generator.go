package gateway

import (
	"gopkg.in/yaml.v3"
)

type gcpBackendPolicyManifest struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   manifestMetadata     `yaml:"metadata"`
	Spec       gcpBackendPolicySpec `yaml:"spec"`
}

type gcpBackendPolicySpec struct {
	Default   gcpBackendPolicyDefault `yaml:"default"`
	TargetRef targetRefDTO            `yaml:"targetRef"`
}

type gcpBackendPolicyDefault struct {
	SecurityPolicy     string                 `yaml:"securityPolicy"`
	TimeoutSec         *int64                 `yaml:"timeoutSec,omitempty"`
	ConnectionDraining *connectionDrainingDTO `yaml:"connectionDraining,omitempty"`
	Logging            *backendLoggingDTO     `yaml:"logging,omitempty"`
}

type connectionDrainingDTO struct {
	DrainingTimeoutSec int64 `yaml:"drainingTimeoutSec"`
}

type backendLoggingDTO struct {
	Enable     bool    `yaml:"enable"`
	SampleRate float64 `yaml:"sampleRate"`
}

type targetRefDTO struct {
	Group string `yaml:"group"`
	Kind  string `yaml:"kind"`
	Name  string `yaml:"name"`
}

type computeSecurityPolicyManifest struct {
	APIVersion string                    `yaml:"apiVersion"`
	Kind       string                    `yaml:"kind"`
	Metadata   manifestMetadata          `yaml:"metadata"`
	Spec       computeSecurityPolicySpec `yaml:"spec"`
}

type computeSecurityPolicySpec struct {
	Description string                   `yaml:"description,omitempty"`
	Rule        []computeSecurityRuleDTO `yaml:"rule"`
}

type computeSecurityRuleDTO struct {
	Action      string               `yaml:"action"`
	Description string               `yaml:"description,omitempty"`
	Match       securityMatchDTO     `yaml:"match"`
	Priority    int32                `yaml:"priority"`
	RateLimit   *rateLimitOptionsDTO `yaml:"rateLimitOptions,omitempty"`
}

type securityMatchDTO struct {
	VersionedExpr string      `yaml:"versionedExpr,omitempty"`
	Config        matchConfig `yaml:"config,omitempty"`
	Expr          *exprDTO    `yaml:"expr,omitempty"`
}

type matchConfig struct {
	SrcIPRanges []string `yaml:"srcIpRanges,omitempty"`
}

type exprDTO struct {
	Expression string `yaml:"expression"`
}

type rateLimitOptionsDTO struct {
	ConformAction      string       `yaml:"conformAction"`
	ExceedAction       string       `yaml:"exceedAction"`
	RateLimitThreshold thresholdDTO `yaml:"rateLimitThreshold"`
}

type thresholdDTO struct {
	Count       int32 `yaml:"count"`
	IntervalSec int32 `yaml:"intervalSec"`
}

type CloudArmorGenerator struct{}

func NewCloudArmorGenerator() *CloudArmorGenerator {
	return &CloudArmorGenerator{}
}

func buildBackendDefault(cfg GCPBackendPolicyConfig) gcpBackendPolicyDefault {
	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = DefaultTimeoutSec
	}
	drainingSec := cfg.ConnectionDrainingTimeoutSec
	if drainingSec <= 0 {
		drainingSec = DefaultConnectionDrainingSec
	}
	sampleRate := cfg.LoggingSampleRate
	if sampleRate <= 0 {
		sampleRate = DefaultLoggingSampleRate
	}
	return gcpBackendPolicyDefault{
		SecurityPolicy: cfg.SecurityPolicyName,
		TimeoutSec:     &timeoutSec,
		ConnectionDraining: &connectionDrainingDTO{
			DrainingTimeoutSec: drainingSec,
		},
		Logging: &backendLoggingDTO{
			Enable:     cfg.LoggingEnabled,
			SampleRate: sampleRate,
		},
	}
}

func buildGCPBackendPolicyManifest(cfg GCPBackendPolicyConfig) gcpBackendPolicyManifest {
	return gcpBackendPolicyManifest{
		APIVersion: GCPBackendPolicyAPIVersion,
		Kind:       GCPBackendPolicyKind,
		Metadata: manifestMetadata{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				LabelAppName:   cfg.TargetServiceName,
				LabelManagedBy: LabelManagedByValue,
			},
		},
		Spec: gcpBackendPolicySpec{
			Default: buildBackendDefault(cfg),
			TargetRef: targetRefDTO{
				Group: "",
				Kind:  TargetKindService,
				Name:  cfg.TargetServiceName,
			},
		},
	}
}

func (g *CloudArmorGenerator) GenerateGCPBackendPolicyYAML(cfg GCPBackendPolicyConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	bytes, err := yaml.Marshal(buildGCPBackendPolicyManifest(cfg))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func buildSecurityMatch(r CloudArmorRule) securityMatchDTO {
	if len(r.SrcIPRanges) > 0 {
		return securityMatchDTO{
			VersionedExpr: VersionedExprSrcIPsV1,
			Config:        matchConfig{SrcIPRanges: r.SrcIPRanges},
		}
	}
	if r.Expression != "" {
		return securityMatchDTO{
			Expr: &exprDTO{Expression: r.Expression},
		}
	}
	return securityMatchDTO{
		VersionedExpr: VersionedExprSrcIPsV1,
		Config:        matchConfig{SrcIPRanges: []string{MatchAnySrcIP}},
	}
}

func buildRateLimitOptions(r CloudArmorRule) *rateLimitOptionsDTO {
	if r.Action != ActionRateBasedBan || r.RateLimitThreshold <= 0 {
		return nil
	}
	intervalSec := r.RateLimitIntervalSec
	if intervalSec <= 0 {
		intervalSec = DefaultRateLimitIntervalSec
	}
	return &rateLimitOptionsDTO{
		ConformAction: ActionAllow,
		ExceedAction:  ActionDeny403,
		RateLimitThreshold: thresholdDTO{
			Count:       r.RateLimitThreshold,
			IntervalSec: intervalSec,
		},
	}
}

func buildComputeSecurityRules(rules []CloudArmorRule) []computeSecurityRuleDTO {
	out := make([]computeSecurityRuleDTO, 0, len(rules))
	for _, r := range rules {
		out = append(out, computeSecurityRuleDTO{
			Action:      r.Action,
			Description: r.Description,
			Priority:    r.Priority,
			Match:       buildSecurityMatch(r),
			RateLimit:   buildRateLimitOptions(r),
		})
	}
	return out
}

func buildComputeSecurityPolicyManifest(cfg ComputeSecurityPolicyConfig) computeSecurityPolicyManifest {
	return computeSecurityPolicyManifest{
		APIVersion: ComputeSecurityPolicyAPIVersion,
		Kind:       ComputeSecurityPolicyKind,
		Metadata: manifestMetadata{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Annotations: map[string]string{
				AnnotationProjectID: cfg.ProjectID,
			},
			Labels: map[string]string{
				LabelAppName:   cfg.Name,
				LabelManagedBy: LabelManagedByValue,
			},
		},
		Spec: computeSecurityPolicySpec{
			Description: cfg.Description,
			Rule:        buildComputeSecurityRules(cfg.Rules),
		},
	}
}

func (g *CloudArmorGenerator) GenerateComputeSecurityPolicyYAML(cfg ComputeSecurityPolicyConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	bytes, err := yaml.Marshal(buildComputeSecurityPolicyManifest(cfg))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
