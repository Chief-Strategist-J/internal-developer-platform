package admission

type PolicySeverity string

const (
	SeverityBlocker  PolicySeverity = "BLOCKER"
	SeverityCritical PolicySeverity = "CRITICAL"
	SeverityStandard PolicySeverity = "STANDARD"
)

const (
	PolicyIDPodSecurityRestricted = "GOV_SEC_001_POD_SECURITY_RESTRICTED"
	PolicyIDMeshInjection         = "GOV_NET_002_MESH_INJECTION_MANDATORY"
	PolicyIDDefaultDenyNetPol     = "GOV_NET_003_DEFAULT_DENY_NETPOL"
	PolicyIDImmutableDigest       = "GOV_IMG_004_IMMUTABLE_IMAGE_DIGEST"
	PolicyIDGovernanceLabels      = "GOV_GOV_005_MANDATORY_GOVERNANCE_LABELS"
	PolicyIDPrivilegedDisallowed  = "GOV_SEC_006_PRIVILEGED_CONTAINER_DISALLOWED"

	PodSecurityLevelRestricted = "restricted"
	PodSecurityLevelBaseline   = "baseline"
	PodSecurityLevelPrivileged = "privileged"

	LabelRequestID  = "platform.gov/request-id"
	LabelOwner      = "platform.gov/owner"
	LabelCostCenter = "platform.gov/cost-center"

	AnnotationMeshInject = "sidecar.istio.io/inject"
	MeshInjectTrue       = "true"

	DigestPrefixSHA256 = "@sha256:"
)

type CELPolicyRule struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Expression     string         `json:"expression"`
	Severity       PolicySeverity `json:"severity"`
	FailureMessage string         `json:"failure_message"`
}

type WorkloadAdmissionInput struct {
	Namespace              string            `json:"namespace"`
	WorkloadName           string            `json:"workload_name"`
	Image                  string            `json:"image"`
	Replicas               int               `json:"replicas"`
	Privileged             bool              `json:"privileged"`
	PodSecurityLevel       string            `json:"pod_security_level"`
	DefaultDenyNetPol      bool              `json:"default_deny_netpol"`
	MeshInjection          bool              `json:"mesh_injection"`
	Labels                 map[string]string `json:"labels"`
	Annotations            map[string]string `json:"annotations"`
}

type ViolationDetail struct {
	PolicyID       string         `json:"policy_id"`
	RuleName       string         `json:"rule_name"`
	Severity       PolicySeverity `json:"severity"`
	FailureMessage string         `json:"failure_message"`
}

type AdmissionEvaluationResult struct {
	Allowed        bool              `json:"allowed"`
	Violations     []ViolationDetail `json:"violations"`
	EvaluatedCount int               `json:"evaluated_count"`
}

var DefaultGovernmentPolicies = []CELPolicyRule{
	{
		ID:             PolicyIDPodSecurityRestricted,
		Name:           "Pod Security Standard Restricted",
		Description:    "Workload namespace must enforce restricted pod security level",
		Expression:     "pod_security_level == 'restricted'",
		Severity:       SeverityBlocker,
		FailureMessage: "pod security level must be strictly 'restricted'",
	},
	{
		ID:             PolicyIDMeshInjection,
		Name:           "Cloud Service Mesh Injection",
		Description:    "Workload must enable managed Cloud Service Mesh sidecar injection",
		Expression:     "mesh_injection == true",
		Severity:       SeverityBlocker,
		FailureMessage: "workload mesh injection must be true",
	},
	{
		ID:             PolicyIDDefaultDenyNetPol,
		Name:           "Default Deny Network Policy",
		Description:    "Workload namespace must enforce default-deny ingress and egress",
		Expression:     "default_deny_netpol == true",
		Severity:       SeverityBlocker,
		FailureMessage: "namespace default-deny network policy is mandatory",
	},
	{
		ID:             PolicyIDImmutableDigest,
		Name:           "Immutable Image Digest",
		Description:    "Container image must be referenced by cryptographic SHA256 digest",
		Expression:     "image.contains('@sha256:')",
		Severity:       SeverityBlocker,
		FailureMessage: "container image must pin an immutable @sha256: digest",
	},
	{
		ID:             PolicyIDGovernanceLabels,
		Name:           "Mandatory Governance Labels",
		Description:    "Workload must carry request-id, owner, and cost-center labels",
		Expression:     "'platform.gov/request-id' in labels && 'platform.gov/owner' in labels && 'platform.gov/cost-center' in labels",
		Severity:       SeverityCritical,
		FailureMessage: "missing mandatory platform.gov governance labels",
	},
	{
		ID:             PolicyIDPrivilegedDisallowed,
		Name:           "Disallow Privileged Containers",
		Description:    "Containers must not run with privileged security context",
		Expression:     "!privileged",
		Severity:       SeverityBlocker,
		FailureMessage: "privileged containers are strictly prohibited in government platform",
	},
}
