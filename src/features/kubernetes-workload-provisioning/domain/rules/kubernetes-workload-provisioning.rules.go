/*
Package rules implements domain validation and admission mutation rules for Kubernetes workload provisioning.
It enforces the CIS GKE Benchmark baseline: restricted Pod Security Standards, non-root execution,
read-only root filesystems, dropped capabilities, automated probes, PodDisruptionBudgets,
and default-deny network isolation.
*/
package rules

import (
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
