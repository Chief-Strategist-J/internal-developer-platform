package gateway

import (
	"strings"

	"internal-developer-platform/src/shared/errors"
)

const (
	GatewayAPIVersion               = "gateway.networking.k8s.io/v1"
	HTTPRouteKind                   = "HTTPRoute"
	GCPBackendPolicyAPIVersion      = "networking.gke.io/v1"
	GCPBackendPolicyKind            = "GCPBackendPolicy"
	ComputeSecurityPolicyAPIVersion = "compute.cnrm.cloud.google.com/v1beta1"
	ComputeSecurityPolicyKind       = "ComputeSecurityPolicy"

	DefaultGatewayGroup        = "gateway.networking.k8s.io"
	DefaultGatewayKind         = "Gateway"
	DefaultGatewayNamespace    = "gateway-system"
	DefaultExternalGatewayName = "gke-external-gateway"
	DefaultInternalGatewayName = "gke-internal-gateway"

	PathMatchPathPrefix = "PathPrefix"
	PathMatchExact      = "Exact"
	DefaultPathPrefix   = "/"

	DefaultPortHTTP = 80
	DefaultPortApp  = 8080

	DefaultSecurityPolicyName    = "platform-edge-security-policy"
	DefaultTimeoutSec            = 30
	DefaultConnectionDrainingSec = 60
	DefaultLoggingSampleRate     = 1.0

	ActionAllow        = "allow"
	ActionDeny403      = "deny(403)"
	ActionRateBasedBan = "rate_based_ban"

	TargetKindService                 = "Service"
	VersionedExprSrcIPsV1             = "SRC_IPS_V1"
	MatchAnySrcIP                     = "*"
	DefaultRateLimitIntervalSec int32 = 60

	LabelAppName        = "app.kubernetes.io/name"
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "internal-developer-platform"
	AnnotationProjectID = "cnrm.cloud.google.com/project-id"

	ErrEmptyNameMsg           = "name must not be empty"
	ErrEmptyNamespaceMsg      = "namespace must not be empty"
	ErrEmptyHostnamesMsg      = "at least one hostname must be provided"
	ErrEmptyServiceNameMsg    = "service name must not be empty"
	ErrInvalidServicePortMsg  = "service port must be greater than zero"
	ErrEmptySecurityPolicyMsg = "security policy name must not be empty"
	ErrEmptyProjectIDMsg      = "project id must not be empty"
	ErrEmptyRulesMsg          = "at least one cloud armor rule must be provided"
	ErrInvalidPriorityMsg     = "rule priority must be greater than or equal to zero"
	ErrInvalidActionMsg       = "rule action must not be empty"
)

type GatewayParentRef struct {
	Group       string `json:"group,omitempty" yaml:"group,omitempty"`
	Kind        string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Namespace   string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Name        string `json:"name" yaml:"name"`
	SectionName string `json:"sectionName,omitempty" yaml:"sectionName,omitempty"`
	Port        *int32 `json:"port,omitempty" yaml:"port,omitempty"`
}

type HTTPRouteMatch struct {
	PathType  string            `json:"pathType" yaml:"pathType"`
	PathValue string            `json:"pathValue" yaml:"pathValue"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

type HTTPRouteBackendRef struct {
	Group  string `json:"group,omitempty" yaml:"group,omitempty"`
	Kind   string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name   string `json:"name" yaml:"name"`
	Port   int32  `json:"port" yaml:"port"`
	Weight *int32 `json:"weight,omitempty" yaml:"weight,omitempty"`
}

type HTTPRouteRule struct {
	Matches     []HTTPRouteMatch      `json:"matches,omitempty" yaml:"matches,omitempty"`
	BackendRefs []HTTPRouteBackendRef `json:"backendRefs,omitempty" yaml:"backendRefs,omitempty"`
}

type HTTPRouteConfig struct {
	Name        string
	Namespace   string
	Hostnames   []string
	ParentRefs  []GatewayParentRef
	Rules       []HTTPRouteRule
	ServiceName string
	ServicePort int32
}

func (c HTTPRouteConfig) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.NewValidationError(ErrEmptyNameMsg)
	}
	if strings.TrimSpace(c.Namespace) == "" {
		return errors.NewValidationError(ErrEmptyNamespaceMsg)
	}
	if len(c.Hostnames) == 0 {
		return errors.NewValidationError(ErrEmptyHostnamesMsg)
	}
	for _, h := range c.Hostnames {
		if strings.TrimSpace(h) == "" {
			return errors.NewValidationError(ErrEmptyHostnamesMsg)
		}
	}
	if strings.TrimSpace(c.ServiceName) == "" {
		return errors.NewValidationError(ErrEmptyServiceNameMsg)
	}
	if c.ServicePort <= 0 {
		return errors.NewValidationError(ErrInvalidServicePortMsg)
	}
	return nil
}

type GCPBackendPolicyConfig struct {
	Name                         string
	Namespace                    string
	TargetServiceName            string
	SecurityPolicyName           string
	TimeoutSec                   int64
	ConnectionDrainingTimeoutSec int64
	LoggingEnabled               bool
	LoggingSampleRate            float64
}

func (c GCPBackendPolicyConfig) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.NewValidationError(ErrEmptyNameMsg)
	}
	if strings.TrimSpace(c.Namespace) == "" {
		return errors.NewValidationError(ErrEmptyNamespaceMsg)
	}
	if strings.TrimSpace(c.TargetServiceName) == "" {
		return errors.NewValidationError(ErrEmptyServiceNameMsg)
	}
	if strings.TrimSpace(c.SecurityPolicyName) == "" {
		return errors.NewValidationError(ErrEmptySecurityPolicyMsg)
	}
	return nil
}

type CloudArmorRule struct {
	Priority             int32    `json:"priority" yaml:"priority"`
	Action               string   `json:"action" yaml:"action"`
	Description          string   `json:"description,omitempty" yaml:"description,omitempty"`
	SrcIPRanges          []string `json:"srcIpRanges,omitempty" yaml:"srcIpRanges,omitempty"`
	Expression           string   `json:"expression,omitempty" yaml:"expression,omitempty"`
	RateLimitThreshold   int32    `json:"rateLimitThreshold,omitempty" yaml:"rateLimitThreshold,omitempty"`
	RateLimitIntervalSec int32    `json:"rateLimitIntervalSec,omitempty" yaml:"rateLimitIntervalSec,omitempty"`
}

type ComputeSecurityPolicyConfig struct {
	Name        string
	Namespace   string
	ProjectID   string
	Description string
	Rules       []CloudArmorRule
}

func (c ComputeSecurityPolicyConfig) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.NewValidationError(ErrEmptyNameMsg)
	}
	if strings.TrimSpace(c.Namespace) == "" {
		return errors.NewValidationError(ErrEmptyNamespaceMsg)
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return errors.NewValidationError(ErrEmptyProjectIDMsg)
	}
	if len(c.Rules) == 0 {
		return errors.NewValidationError(ErrEmptyRulesMsg)
	}
	for _, r := range c.Rules {
		if r.Priority < 0 {
			return errors.NewValidationError(ErrInvalidPriorityMsg)
		}
		if strings.TrimSpace(r.Action) == "" {
			return errors.NewValidationError(ErrInvalidActionMsg)
		}
	}
	return nil
}
