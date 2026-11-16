package rules

import (
	"fmt"
	"strconv"
	"strings"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/shared/errors"
)

type SecurityContextDefaults struct {
	RunAsNonRoot             bool
	RunAsUser                int64
	RunAsGroup               int64
	ReadOnlyRootFilesystem   bool
	AllowPrivilegeEscalation bool
	DroppedCapabilities      []string
	SeccompProfileType       string
}

type ProbeDefaults struct {
	StartupPath   string
	LivenessPath  string
	ReadinessPath string
	Port          int
}

type ResourceQuotaDefaults struct {
	RequestsCPU    string
	RequestsMemory string
	LimitsCPU      string
	LimitsMemory   string
	MaxPods        string
}

type LimitRangeDefaults struct {
	DefaultLimitCPU      string
	DefaultLimitMemory   string
	DefaultRequestCPU    string
	DefaultRequestMemory string
}

type WorkloadMutations struct {
	SecurityContext SecurityContextDefaults
	Probes          ProbeDefaults
	PDBRequired     bool
	MinAvailable    int
	CPURequest      string
	CPULimit        string
	MemoryRequest   string
	MemoryLimit     string
	NamespaceLabels map[string]string
	ResourceQuota   ResourceQuotaDefaults
	LimitRange      LimitRangeDefaults
}

func EvaluateWorkloadMutations(cmd contracts.WorkloadProvisionCommand) WorkloadMutations {
	cpuReq := cmd.CPURequest
	if cpuReq == "" {
		cpuReq = "250m"
	}

	cpuLim := cmd.CPULimit
	if cpuLim == "" {
		cpuLim = "1000m"
	}

	memReq := cmd.MemoryRequest
	if memReq == "" {
		memReq = "256Mi"
	}

	memLim := cmd.MemoryLimit
	if memLim == "" {
		memLim = "1Gi"
	}

	pdbRequired := cmd.Replicas >= 2
	minAvailable := 0
	if pdbRequired {
		minAvailable = 1
	}

	labels := map[string]string{
		"pod-security.kubernetes.io/enforce": "restricted",
		"pod-security.kubernetes.io/audit":   "restricted",
		"pod-security.kubernetes.io/warn":    "restricted",
		"istio.io/rev":                       "asm-managed",
		"app.kubernetes.io/managed-by":       "internal-developer-platform",
		"app.kubernetes.io/part-of":          cmd.TenantID,
		"app.kubernetes.io/name":             cmd.ServiceID,
	}

	quota := ResourceQuotaDefaults{
		RequestsCPU:    "10",
		RequestsMemory: "20Gi",
		LimitsCPU:      "20",
		LimitsMemory:   "40Gi",
		MaxPods:        "50",
	}

	limitRange := LimitRangeDefaults{
		DefaultLimitCPU:      "1000m",
		DefaultLimitMemory:   "1Gi",
		DefaultRequestCPU:    "250m",
		DefaultRequestMemory: "256Mi",
	}

	return WorkloadMutations{
		SecurityContext: SecurityContextDefaults{
			RunAsNonRoot:             true,
			RunAsUser:                10001,
			RunAsGroup:               10001,
			ReadOnlyRootFilesystem:   true,
			AllowPrivilegeEscalation: false,
			DroppedCapabilities:      []string{"ALL"},
			SeccompProfileType:       "RuntimeDefault",
		},
		Probes: ProbeDefaults{
			StartupPath:   "/readyz",
			LivenessPath:  "/livez",
			ReadinessPath: "/readyz",
			Port:          8080,
		},
		PDBRequired:     pdbRequired,
		MinAvailable:    minAvailable,
		CPURequest:      cpuReq,
		CPULimit:        cpuLim,
		MemoryRequest:   memReq,
		MemoryLimit:     memLim,
		NamespaceLabels: labels,
		ResourceQuota:   quota,
		LimitRange:      limitRange,
	}
}

func ValidateAdmission(cmd contracts.WorkloadProvisionCommand, mutations WorkloadMutations) (contracts.AdmissionReport, error) {
	violations := make([]contracts.AdmissionViolation, 0)
	critical := false

	if !mutations.SecurityContext.RunAsNonRoot || mutations.SecurityContext.RunAsUser == 0 {
		critical = true
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "SEC-001",
			Message: "Workload must execute as non-root user UID > 0",
		})
	}

	if mutations.SecurityContext.AllowPrivilegeEscalation {
		critical = true
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "SEC-002",
			Message: "Privilege escalation is strictly forbidden",
		})
	}

	if !mutations.SecurityContext.ReadOnlyRootFilesystem {
		critical = true
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "SEC-003",
			Message: "Root filesystem must be read-only",
		})
	}

	if cmd.Replicas >= 2 && !mutations.PDBRequired {
		critical = true
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "REL-001",
			Message: "Multi-replica workloads require PodDisruptionBudget",
		})
	}

	if strings.HasSuffix(cmd.Image, ":latest") {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "IMG-001",
			Message: "Warning: Mutable image tag :latest detected",
		})
	}

	if critical {
		return contracts.AdmissionReport{
			Admitted:   false,
			Violations: violations,
		}, errors.NewValidationError("workload admission validation failed: critical security violations")
	}

	return contracts.AdmissionReport{
		Admitted:   true,
		Violations: violations,
	}, nil
}

func ParseMillicores(val string) (int64, error) {
	clean := strings.TrimSpace(val)
	if clean == "" {
		return 0, nil
	}
	if strings.HasSuffix(clean, "m") {
		numStr := strings.TrimSuffix(clean, "m")
		return strconv.ParseInt(numStr, 10, 64)
	}
	cores, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, err
	}
	return int64(cores * 1000), nil
}

func ParseMegabytes(val string) (int64, error) {
	clean := strings.TrimSpace(val)
	if clean == "" {
		return 0, nil
	}
	if strings.HasSuffix(clean, "Gi") {
		numStr := strings.TrimSuffix(clean, "Gi")
		gi, err := strconv.ParseInt(numStr, 10, 64)
		return gi * 1024, err
	}
	if strings.HasSuffix(clean, "G") {
		numStr := strings.TrimSuffix(clean, "G")
		g, err := strconv.ParseInt(numStr, 10, 64)
		return g * 1000, err
	}
	if strings.HasSuffix(clean, "Mi") {
		numStr := strings.TrimSuffix(clean, "Mi")
		return strconv.ParseInt(numStr, 10, 64)
	}
	if strings.HasSuffix(clean, "M") {
		numStr := strings.TrimSuffix(clean, "M")
		return strconv.ParseInt(numStr, 10, 64)
	}
	bytes, err := strconv.ParseInt(clean, 10, 64)
	if err != nil {
		return 0, err
	}
	return bytes / (1024 * 1024), nil
}

func EvaluateResourceQuotaHeadroom(quota contracts.ResourceQuotaSpec, requestedCPU string, requestedMemory string, replicas int) contracts.QuotaCheckResult {
	if replicas <= 0 {
		replicas = 1
	}

	hardCPU, err := ParseMillicores(quota.HardCPU)
	if err != nil || hardCPU <= 0 {
		return contracts.QuotaCheckResult{SufficientHeadroom: true}
	}
	usedCPU, _ := ParseMillicores(quota.UsedCPU)

	hardMem, err := ParseMegabytes(quota.HardMemory)
	if err != nil || hardMem <= 0 {
		return contracts.QuotaCheckResult{SufficientHeadroom: true}
	}
	usedMem, _ := ParseMegabytes(quota.UsedMemory)

	availCPU := hardCPU - usedCPU
	availMem := hardMem - usedMem

	reqCPUMilli, err := ParseMillicores(requestedCPU)
	if err != nil {
		reqCPUMilli = 250
	}
	totalReqCPU := reqCPUMilli * int64(replicas)

	reqMemMB, err := ParseMegabytes(requestedMemory)
	if err != nil {
		reqMemMB = 256
	}
	totalReqMem := reqMemMB * int64(replicas)

	if totalReqCPU > availCPU {
		return contracts.QuotaCheckResult{
			SufficientHeadroom: false,
			AvailableCPU:       fmt.Sprintf("%dm", availCPU),
			AvailableMemory:    fmt.Sprintf("%dMi", availMem),
			ViolationMessage:   fmt.Sprintf("quota exceeded: requested cpu %dm exceeds available %dm", totalReqCPU, availCPU),
		}
	}

	if totalReqMem > availMem {
		return contracts.QuotaCheckResult{
			SufficientHeadroom: false,
			AvailableCPU:       fmt.Sprintf("%dm", availCPU),
			AvailableMemory:    fmt.Sprintf("%dMi", availMem),
			ViolationMessage:   fmt.Sprintf("quota exceeded: requested memory %dMi exceeds available %dMi", totalReqMem, availMem),
		}
	}

	return contracts.QuotaCheckResult{
		SufficientHeadroom: true,
		AvailableCPU:       fmt.Sprintf("%dm", availCPU),
		AvailableMemory:    fmt.Sprintf("%dMi", availMem),
	}
}

func GenerateArgoCDApplicationYAML(cmd contracts.WorkloadProvisionCommand, appConfig contracts.GitOpsAppConfig) string {
	project := appConfig.ProjectName
	if project == "" {
		project = "default"
	}
	revision := appConfig.TargetRevision
	if revision == "" {
		revision = "HEAD"
	}
	server := appConfig.ServerURL
	if server == "" {
		server = "https://kubernetes.default.svc"
	}
	repo := appConfig.RepoURL
	if repo == "" {
		repo = "https://github.com/Chief-Strategist-J/platform-gitops-manifests.git"
	}
	path := appConfig.Path
	if path == "" {
		path = fmt.Sprintf("workloads/%s/%s", cmd.Namespace, cmd.WorkloadName)
	}

	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: argocd
  labels:
    app.kubernetes.io/managed-by: internal-developer-platform
    tenant.platform.internal/id: %s
spec:
  project: %s
  source:
    repoURL: %s
    targetRevision: %s
    path: %s
  destination:
    server: %s
    namespace: %s
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - RespectIgnoreDifferences=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
  ignoreDifferences:
    - group: apps
      kind: Deployment
      jsonPointers:
        - /spec/replicas
`, cmd.WorkloadName, cmd.TenantID, project, repo, revision, path, server, cmd.Namespace)
}
