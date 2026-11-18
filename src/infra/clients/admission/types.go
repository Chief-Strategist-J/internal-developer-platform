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

	RuleNamePodSecurityRestricted = "Pod Security Standard Restricted"
	RuleNameMeshInjection         = "Cloud Service Mesh Injection"
	RuleNameDefaultDenyNetPol     = "Default Deny Network Policy"
	RuleNameImmutableDigest       = "Immutable Image Digest"
	RuleNameGovernanceLabels      = "Mandatory Governance Labels"
	RuleNamePrivilegedDisallowed  = "Disallow Privileged Containers"

	RuleDescPodSecurityRestricted = "Workload namespace must enforce restricted pod security level"
	RuleDescMeshInjection         = "Workload must enable managed Cloud Service Mesh sidecar injection"
	RuleDescDefaultDenyNetPol     = "Workload namespace must enforce default-deny ingress and egress"
	RuleDescImmutableDigest       = "Container image must be referenced by cryptographic SHA256 digest"
	RuleDescGovernanceLabels      = "Workload must carry request-id, owner, and cost-center labels"
	RuleDescPrivilegedDisallowed  = "Containers must not run with privileged security context"

	ExprPodSecurityRestricted = "pod_security_level == 'restricted'"
	ExprMeshInjection         = "mesh_injection == true"
	ExprDefaultDenyNetPol     = "default_deny_netpol == true"
	ExprImmutableDigest       = "image.contains('@sha256:')"
	ExprGovernanceLabels      = "'platform.gov/request-id' in labels && 'platform.gov/owner' in labels && 'platform.gov/cost-center' in labels"
	ExprPrivilegedDisallowed  = "!privileged"

	MsgErrPodSecurityRestricted = "pod security level must be strictly 'restricted'"
	MsgErrMeshInjection         = "workload mesh injection must be true"
	MsgErrDefaultDenyNetPol     = "namespace default-deny network policy is mandatory"
	MsgErrImmutableDigest       = "container image must pin an immutable @sha256: digest"
	MsgErrGovernanceLabels      = "missing mandatory platform.gov governance labels"
	MsgErrPrivilegedDisallowed  = "privileged containers are strictly prohibited in government platform"

	PodSecurityLevelRestricted = "restricted"
	PodSecurityLevelBaseline   = "baseline"
	PodSecurityLevelPrivileged = "privileged"

	LabelRequestID  = "platform.gov/request-id"
	LabelOwner      = "platform.gov/owner"
	LabelCostCenter = "platform.gov/cost-center"

	AnnotationMeshInject = "sidecar.istio.io/inject"
	MeshInjectTrue       = "true"

	DigestPrefixSHA256 = "@sha256:"

	VarPodSecurityLevel = "pod_security_level"
	VarMeshInjection    = "mesh_injection"
	VarDefaultDenyNetPol = "default_deny_netpol"
	VarImage            = "image"
	VarPrivileged       = "privileged"
	VarLabels           = "labels"
	VarAnnotations      = "annotations"

	SpanAdmissionEvaluate        = "kubernetes.workload.admission.cel_evaluate"
	AttrWorkloadName             = "workload.name"
	AttrNamespace                = "namespace"
	AttrAdmissionAllowed         = "admission.allowed"
	AttrAdmissionViolationsCount = "admission.violations.count"
	AttrAdmissionEvaluatedCount  = "admission.evaluated.count"

	ErrMsgEvalPrefix  = "evaluation error for rule %s: %v"
	ErrMsgEvalFailure = "evaluation error: %v"
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
	Namespace         string            `json:"namespace"`
	WorkloadName      string            `json:"workload_name"`
	Image             string            `json:"image"`
	Replicas          int               `json:"replicas"`
	Privileged        bool              `json:"privileged"`
	PodSecurityLevel  string            `json:"pod_security_level"`
	DefaultDenyNetPol bool              `json:"default_deny_netpol"`
	MeshInjection     bool              `json:"mesh_injection"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
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
		Name:           RuleNamePodSecurityRestricted,
		Description:    RuleDescPodSecurityRestricted,
		Expression:     ExprPodSecurityRestricted,
		Severity:       SeverityBlocker,
		FailureMessage: MsgErrPodSecurityRestricted,
	},
	{
		ID:             PolicyIDMeshInjection,
		Name:           RuleNameMeshInjection,
		Description:    RuleDescMeshInjection,
		Expression:     ExprMeshInjection,
		Severity:       SeverityBlocker,
		FailureMessage: MsgErrMeshInjection,
	},
	{
		ID:             PolicyIDDefaultDenyNetPol,
		Name:           RuleNameDefaultDenyNetPol,
		Description:    RuleDescDefaultDenyNetPol,
		Expression:     ExprDefaultDenyNetPol,
		Severity:       SeverityBlocker,
		FailureMessage: MsgErrDefaultDenyNetPol,
	},
	{
		ID:             PolicyIDImmutableDigest,
		Name:           RuleNameImmutableDigest,
		Description:    RuleDescImmutableDigest,
		Expression:     ExprImmutableDigest,
		Severity:       SeverityBlocker,
		FailureMessage: MsgErrImmutableDigest,
	},
	{
		ID:             PolicyIDGovernanceLabels,
		Name:           RuleNameGovernanceLabels,
		Description:    RuleDescGovernanceLabels,
		Expression:     ExprGovernanceLabels,
		Severity:       SeverityCritical,
		FailureMessage: MsgErrGovernanceLabels,
	},
	{
		ID:             PolicyIDPrivilegedDisallowed,
		Name:           RuleNamePrivilegedDisallowed,
		Description:    RuleDescPrivilegedDisallowed,
		Expression:     ExprPrivilegedDisallowed,
		Severity:       SeverityBlocker,
		FailureMessage: MsgErrPrivilegedDisallowed,
	},
}
