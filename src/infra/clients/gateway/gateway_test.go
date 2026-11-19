package gateway_test

import (
	"strings"
	"testing"

	"internal-developer-platform/src/infra/clients/gateway"
)

func TestHTTPRouteGenerator_DefaultManifest(t *testing.T) {
	gen := gateway.NewHTTPRouteGenerator()
	cfg := gateway.HTTPRouteConfig{
		Name:        "order-service",
		Namespace:   "tenant-retail",
		Hostnames:   []string{"order.retail.example.com"},
		ServiceName: "order-service",
		ServicePort: 8080,
	}

	yamlStr, err := gen.GenerateHTTPRouteYAML(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(yamlStr, "apiVersion: gateway.networking.k8s.io/v1") {
		t.Errorf("missing apiVersion in YAML")
	}
	if !strings.Contains(yamlStr, "kind: HTTPRoute") {
		t.Errorf("missing kind HTTPRoute in YAML")
	}
	if !strings.Contains(yamlStr, "name: order-service") {
		t.Errorf("missing workload name in YAML")
	}
	if !strings.Contains(yamlStr, "namespace: gateway-system") {
		t.Errorf("missing default gateway-system parentRef in YAML")
	}
	if !strings.Contains(yamlStr, "name: gke-external-gateway") {
		t.Errorf("missing default gke-external-gateway in YAML")
	}
	if !strings.Contains(yamlStr, "order.retail.example.com") {
		t.Errorf("missing hostname in YAML")
	}
	if !strings.Contains(yamlStr, "port: 8080") {
		t.Errorf("missing backend port in YAML")
	}
}

func TestHTTPRouteGenerator_CustomRulesAndParentRef(t *testing.T) {
	gen := gateway.NewHTTPRouteGenerator()
	weight := int32(80)
	cfg := gateway.HTTPRouteConfig{
		Name:      "api-gateway-route",
		Namespace: "tenant-fintech",
		Hostnames: []string{"api.fintech.example.com"},
		ParentRefs: []gateway.GatewayParentRef{
			{
				Group:     gateway.DefaultGatewayGroup,
				Kind:      gateway.DefaultGatewayKind,
				Namespace: "custom-gateway-ns",
				Name:      "internal-edge-gw",
			},
		},
		Rules: []gateway.HTTPRouteRule{
			{
				Matches: []gateway.HTTPRouteMatch{
					{
						PathType:  gateway.PathMatchPathPrefix,
						PathValue: "/v1/payments",
					},
				},
				BackendRefs: []gateway.HTTPRouteBackendRef{
					{
						Name:   "payment-svc",
						Port:   9090,
						Weight: &weight,
					},
				},
			},
		},
		ServiceName: "payment-svc",
		ServicePort: 9090,
	}

	yamlStr, err := gen.GenerateHTTPRouteYAML(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(yamlStr, "name: internal-edge-gw") {
		t.Errorf("missing custom gateway name")
	}
	if !strings.Contains(yamlStr, "namespace: custom-gateway-ns") {
		t.Errorf("missing custom gateway namespace")
	}
	if !strings.Contains(yamlStr, "value: /v1/payments") {
		t.Errorf("missing custom path prefix")
	}
	if !strings.Contains(yamlStr, "weight: 80") {
		t.Errorf("missing custom weight")
	}
}

func TestHTTPRouteGenerator_ValidationErrors(t *testing.T) {
	gen := gateway.NewHTTPRouteGenerator()

	cases := []struct {
		name string
		cfg  gateway.HTTPRouteConfig
	}{
		{
			name: "empty name",
			cfg: gateway.HTTPRouteConfig{
				Namespace:   "default",
				Hostnames:   []string{"example.com"},
				ServiceName: "svc",
				ServicePort: 80,
			},
		},
		{
			name: "empty namespace",
			cfg: gateway.HTTPRouteConfig{
				Name:        "test",
				Hostnames:   []string{"example.com"},
				ServiceName: "svc",
				ServicePort: 80,
			},
		},
		{
			name: "empty hostnames",
			cfg: gateway.HTTPRouteConfig{
				Name:        "test",
				Namespace:   "default",
				Hostnames:   []string{},
				ServiceName: "svc",
				ServicePort: 80,
			},
		},
		{
			name: "empty service name",
			cfg: gateway.HTTPRouteConfig{
				Name:        "test",
				Namespace:   "default",
				Hostnames:   []string{"example.com"},
				ServiceName: "",
				ServicePort: 80,
			},
		},
		{
			name: "invalid service port",
			cfg: gateway.HTTPRouteConfig{
				Name:        "test",
				Namespace:   "default",
				Hostnames:   []string{"example.com"},
				ServiceName: "svc",
				ServicePort: 0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gen.GenerateHTTPRouteYAML(tc.cfg)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tc.name)
			}
		})
	}
}

func TestCloudArmorGenerator_GCPBackendPolicySuccess(t *testing.T) {
	gen := gateway.NewCloudArmorGenerator()
	cfg := gateway.GCPBackendPolicyConfig{
		Name:                         "order-service-policy",
		Namespace:                    "tenant-retail",
		TargetServiceName:            "order-service",
		SecurityPolicyName:           "platform-edge-security-policy",
		TimeoutSec:                   45,
		ConnectionDrainingTimeoutSec: 120,
		LoggingEnabled:               true,
		LoggingSampleRate:            0.5,
	}

	yamlStr, err := gen.GenerateGCPBackendPolicyYAML(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(yamlStr, "apiVersion: networking.gke.io/v1") {
		t.Errorf("missing apiVersion in BackendPolicy")
	}
	if !strings.Contains(yamlStr, "kind: GCPBackendPolicy") {
		t.Errorf("missing kind GCPBackendPolicy")
	}
	if !strings.Contains(yamlStr, "securityPolicy: platform-edge-security-policy") {
		t.Errorf("missing securityPolicy reference")
	}
	if !strings.Contains(yamlStr, "timeoutSec: 45") {
		t.Errorf("missing custom timeoutSec")
	}
	if !strings.Contains(yamlStr, "drainingTimeoutSec: 120") {
		t.Errorf("missing drainingTimeoutSec")
	}
	if !strings.Contains(yamlStr, "name: order-service") {
		t.Errorf("missing targetRef service name")
	}
}

func TestCloudArmorGenerator_GCPBackendPolicyValidation(t *testing.T) {
	gen := gateway.NewCloudArmorGenerator()

	cfg := gateway.GCPBackendPolicyConfig{
		Name:               "",
		Namespace:          "default",
		TargetServiceName:  "order-svc",
		SecurityPolicyName: "policy",
	}

	if _, err := gen.GenerateGCPBackendPolicyYAML(cfg); err == nil {
		t.Fatalf("expected error for empty name")
	}

	cfg.Name = "valid"
	cfg.SecurityPolicyName = ""
	if _, err := gen.GenerateGCPBackendPolicyYAML(cfg); err == nil {
		t.Fatalf("expected error for empty security policy")
	}
}

func TestCloudArmorGenerator_ComputeSecurityPolicySuccess(t *testing.T) {
	gen := gateway.NewCloudArmorGenerator()
	cfg := gateway.ComputeSecurityPolicyConfig{
		Name:        "waf-armor-policy",
		Namespace:   "infra-security",
		ProjectID:   "gcp-prod-fleet-01",
		Description: "Production OWASP and Rate Limit Protection",
		Rules: []gateway.CloudArmorRule{
			{
				Priority:    1000,
				Action:      gateway.ActionAllow,
				Description: "Allow corporate office IPs",
				SrcIPRanges: []string{"198.51.100.0/24", "203.0.113.0/24"},
			},
			{
				Priority:             2000,
				Action:               gateway.ActionRateBasedBan,
				Description:          "Rate limit per client IP",
				SrcIPRanges:          []string{"*"},
				RateLimitThreshold:   500,
				RateLimitIntervalSec: 60,
			},
			{
				Priority:    2147483647,
				Action:      gateway.ActionDeny403,
				Description: "Default deny all other traffic",
				SrcIPRanges: []string{"*"},
			},
		},
	}

	yamlStr, err := gen.GenerateComputeSecurityPolicyYAML(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(yamlStr, "apiVersion: compute.cnrm.cloud.google.com/v1beta1") {
		t.Errorf("missing compute cnrm apiVersion")
	}
	if !strings.Contains(yamlStr, "kind: ComputeSecurityPolicy") {
		t.Errorf("missing ComputeSecurityPolicy kind")
	}
	if !strings.Contains(yamlStr, "cnrm.cloud.google.com/project-id: gcp-prod-fleet-01") {
		t.Errorf("missing project ID annotation")
	}
	if !strings.Contains(yamlStr, "action: rate_based_ban") {
		t.Errorf("missing rate_based_ban action")
	}
	if !strings.Contains(yamlStr, "count: 500") {
		t.Errorf("missing rate limit threshold count")
	}
	if !strings.Contains(yamlStr, "intervalSec: 60") {
		t.Errorf("missing rate limit interval")
	}
}

func TestCloudArmorGenerator_ComputeSecurityPolicyValidation(t *testing.T) {
	gen := gateway.NewCloudArmorGenerator()

	cfg := gateway.ComputeSecurityPolicyConfig{
		Name:      "waf",
		Namespace: "default",
		ProjectID: "",
		Rules: []gateway.CloudArmorRule{
			{Priority: 100, Action: "allow"},
		},
	}

	if _, err := gen.GenerateComputeSecurityPolicyYAML(cfg); err == nil {
		t.Fatalf("expected error for empty project id")
	}

	cfg.ProjectID = "proj"
	cfg.Rules = []gateway.CloudArmorRule{}
	if _, err := gen.GenerateComputeSecurityPolicyYAML(cfg); err == nil {
		t.Fatalf("expected error for empty rules")
	}
}

