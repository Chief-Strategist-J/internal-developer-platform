package gateway

import (
	"strings"

	"gopkg.in/yaml.v3"
)

type httpRouteSpec struct {
	ParentRefs []parentRefDTO `yaml:"parentRefs"`
	Hostnames  []string       `yaml:"hostnames"`
	Rules      []ruleDTO      `yaml:"rules"`
}

type parentRefDTO struct {
	Group       string `yaml:"group,omitempty"`
	Kind        string `yaml:"kind,omitempty"`
	Namespace   string `yaml:"namespace,omitempty"`
	Name        string `yaml:"name"`
	SectionName string `yaml:"sectionName,omitempty"`
	Port        *int32 `yaml:"port,omitempty"`
}

type ruleDTO struct {
	Matches     []matchDTO      `yaml:"matches,omitempty"`
	BackendRefs []backendRefDTO `yaml:"backendRefs"`
}

type matchDTO struct {
	Path *pathMatchDTO `yaml:"path,omitempty"`
}

type pathMatchDTO struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

type backendRefDTO struct {
	Group  string `yaml:"group,omitempty"`
	Kind   string `yaml:"kind,omitempty"`
	Name   string `yaml:"name"`
	Port   int32  `yaml:"port"`
	Weight *int32 `yaml:"weight,omitempty"`
}

type httpRouteManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   manifestMetadata `yaml:"metadata"`
	Spec       httpRouteSpec    `yaml:"spec"`
}

type manifestMetadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type HTTPRouteGenerator struct{}

func NewHTTPRouteGenerator() *HTTPRouteGenerator {
	return &HTTPRouteGenerator{}
}

func buildParentRefs(cfg HTTPRouteConfig) []parentRefDTO {
	if len(cfg.ParentRefs) == 0 {
		return []parentRefDTO{
			{
				Group:     DefaultGatewayGroup,
				Kind:      DefaultGatewayKind,
				Namespace: DefaultGatewayNamespace,
				Name:      DefaultExternalGatewayName,
			},
		}
	}
	parentRefs := make([]parentRefDTO, 0, len(cfg.ParentRefs))
	for _, p := range cfg.ParentRefs {
		parentRefs = append(parentRefs, parentRefDTO{
			Group:       p.Group,
			Kind:        p.Kind,
			Namespace:   p.Namespace,
			Name:        p.Name,
			SectionName: p.SectionName,
			Port:        p.Port,
		})
	}
	return parentRefs
}

func buildDefaultRules(cfg HTTPRouteConfig) []ruleDTO {
	defaultWeight := int32(100)
	return []ruleDTO{
		{
			Matches: []matchDTO{
				{
					Path: &pathMatchDTO{
						Type:  PathMatchPathPrefix,
						Value: DefaultPathPrefix,
					},
				},
			},
			BackendRefs: []backendRefDTO{
				{
					Group:  "",
					Kind:   TargetKindService,
					Name:   cfg.ServiceName,
					Port:   cfg.ServicePort,
					Weight: &defaultWeight,
				},
			},
		},
	}
}

func buildCustomMatch(m HTTPRouteMatch) matchDTO {
	pathType := m.PathType
	if strings.TrimSpace(pathType) == "" {
		pathType = PathMatchPathPrefix
	}
	pathValue := m.PathValue
	if strings.TrimSpace(pathValue) == "" {
		pathValue = DefaultPathPrefix
	}
	return matchDTO{
		Path: &pathMatchDTO{
			Type:  pathType,
			Value: pathValue,
		},
	}
}

func buildCustomBackend(b HTTPRouteBackendRef) backendRefDTO {
	kind := b.Kind
	if strings.TrimSpace(kind) == "" {
		kind = TargetKindService
	}
	return backendRefDTO{
		Group:  b.Group,
		Kind:   kind,
		Name:   b.Name,
		Port:   b.Port,
		Weight: b.Weight,
	}
}

func buildCustomRules(rules []HTTPRouteRule) []ruleDTO {
	out := make([]ruleDTO, 0, len(rules))
	for _, r := range rules {
		rule := ruleDTO{
			Matches:     make([]matchDTO, 0, len(r.Matches)),
			BackendRefs: make([]backendRefDTO, 0, len(r.BackendRefs)),
		}
		for _, m := range r.Matches {
			rule.Matches = append(rule.Matches, buildCustomMatch(m))
		}
		for _, b := range r.BackendRefs {
			rule.BackendRefs = append(rule.BackendRefs, buildCustomBackend(b))
		}
		out = append(out, rule)
	}
	return out
}

func buildHTTPRouteManifest(cfg HTTPRouteConfig, rules []ruleDTO) httpRouteManifest {
	return httpRouteManifest{
		APIVersion: GatewayAPIVersion,
		Kind:       HTTPRouteKind,
		Metadata: manifestMetadata{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				LabelAppName:   cfg.Name,
				LabelManagedBy: LabelManagedByValue,
			},
		},
		Spec: httpRouteSpec{
			ParentRefs: buildParentRefs(cfg),
			Hostnames:  cfg.Hostnames,
			Rules:      rules,
		},
	}
}

func (g *HTTPRouteGenerator) GenerateHTTPRouteYAML(cfg HTTPRouteConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	rules := buildDefaultRules(cfg)
	if len(cfg.Rules) > 0 {
		rules = buildCustomRules(cfg.Rules)
	}
	bytes, err := yaml.Marshal(buildHTTPRouteManifest(cfg, rules))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
