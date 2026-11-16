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


func EvaluateDefaultRolloutConfig(cmd contracts.WorkloadProvisionCommand) contracts.RolloutConfig {
	strategy := cmd.Strategy
	if strategy == "" {
		strategy = contracts.StrategyCanary
	}

	return contracts.RolloutConfig{
		Strategy: strategy,
		CanarySteps: []contracts.CanaryStep{
			{SetWeight: 10, PauseDurationSeconds: 60},
			{SetWeight: 25, PauseDurationSeconds: 120},
			{SetWeight: 50, PauseDurationSeconds: 180},
			{SetWeight: 100, PauseDurationSeconds: 0},
		},
		AnalysisMetrics: []contracts.AnalysisMetric{
			{
				Name:             "success-rate",
				Interval:         "30s",
				SuccessCondition: "result[0] >= 0.99",
				FailureLimit:     3,
				Query:            `sum(rate(istio_requests_total{reporter="destination",response_code!~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination"}[1m]))`,
			},
			{
				Name:             "latency-p99",
				Interval:         "30s",
				SuccessCondition: "result[0] <= 500",
				FailureLimit:     3,
				Query:            `histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{reporter="destination"}[1m])) by (le))`,
			},
		},
	}
}

func GenerateAnalysisTemplateYAML(cmd contracts.WorkloadProvisionCommand, metrics []contracts.AnalysisMetric) string {
	var metricsBuilder strings.Builder
	for _, m := range metrics {
		metricsBuilder.WriteString(fmt.Sprintf(`    - name: %s
      interval: %s
      successCondition: %s
      failureLimit: %d
      provider:
        prometheus:
          address: http://prometheus-k8s.monitoring.svc:9090
          query: %s
`, m.Name, m.Interval, m.SuccessCondition, m.FailureLimit, m.Query))
	}

	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: %s-canary-analysis
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/part-of: %s
spec:
  metrics:
%s`, cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, cmd.TenantID, metricsBuilder.String())
}

func GenerateArgoRolloutYAML(cmd contracts.WorkloadProvisionCommand, mutations WorkloadMutations, rolloutCfg contracts.RolloutConfig) string {
	var stepsBuilder strings.Builder
	for _, step := range rolloutCfg.CanarySteps {
		stepsBuilder.WriteString(fmt.Sprintf(`        - setWeight: %d
`, step.SetWeight))
		if step.PauseDurationSeconds > 0 {
			stepsBuilder.WriteString(fmt.Sprintf(`        - pause: { duration: %ds }
`, step.PauseDurationSeconds))
		}
	}

	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/part-of: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  strategy:
    canary:
      analysis:
        templates:
          - templateName: %s-canary-analysis
        args:
          - name: service-name
            value: %s
      steps:
%s  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
        app.kubernetes.io/part-of: %s
    spec:
      securityContext:
        runAsNonRoot: %t
        runAsUser: %d
        runAsGroup: %d
        seccompProfile:
          type: %s
      containers:
      - name: %s
        image: %s
        securityContext:
          allowPrivilegeEscalation: %t
          readOnlyRootFilesystem: %t
          capabilities:
            drop:
            - ALL
        resources:
          requests:
            cpu: "%s"
            memory: "%s"
          limits:
            cpu: "%s"
            memory: "%s"
        startupProbe:
          httpGet:
            path: %s
            port: %d
        livenessProbe:
          httpGet:
            path: %s
            port: %d
        readinessProbe:
          httpGet:
            path: %s
            port: %d
`,
		cmd.WorkloadName,
		cmd.Namespace,
		cmd.WorkloadName,
		cmd.TenantID,
		cmd.Replicas,
		cmd.WorkloadName,
		cmd.WorkloadName,
		cmd.WorkloadName,
		stepsBuilder.String(),
		cmd.WorkloadName,
		cmd.TenantID,
		mutations.SecurityContext.RunAsNonRoot,
		mutations.SecurityContext.RunAsUser,
		mutations.SecurityContext.RunAsGroup,
		mutations.SecurityContext.SeccompProfileType,
		cmd.WorkloadName,
		cmd.Image,
		mutations.SecurityContext.AllowPrivilegeEscalation,
		mutations.SecurityContext.ReadOnlyRootFilesystem,
		mutations.CPURequest,
		mutations.MemoryRequest,
		mutations.CPULimit,
		mutations.MemoryLimit,
		mutations.Probes.StartupPath,
		mutations.Probes.Port,
		mutations.Probes.LivenessPath,
		mutations.Probes.Port,
		mutations.Probes.ReadinessPath,
		mutations.Probes.Port,
	)
}

func GenerateMeshAuthorizationPolicyYAML(cmd contracts.WorkloadProvisionCommand, secCfg contracts.MeshSecurityConfig) string {
	mode := secCfg.MtlsMode
	if mode == "" {
		mode = "STRICT"
	}

	principal := fmt.Sprintf("cluster.local/ns/%s/sa/%s", cmd.Namespace, cmd.WorkloadName)
	principals := []string{principal}
	for _, p := range secCfg.AllowedPrincipals {
		if p != "" && p != principal {
			principals = append(principals, p)
		}
	}

	methods := secCfg.AllowedMethods
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "DELETE"}
	}

	var principalsBuilder strings.Builder
	for _, p := range principals {
		principalsBuilder.WriteString(fmt.Sprintf(`        - "%s"
`, p))
	}

	var methodsBuilder strings.Builder
	for _, m := range methods {
		methodsBuilder.WriteString(fmt.Sprintf(`        - "%s"
`, m))
	}

	return fmt.Sprintf(`apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: %s-authz
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/part-of: %s
    security.istio.io/mtls: "%s"
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  action: ALLOW
  rules:
  - from:
    - source:
        principals:
%s    to:
    - operation:
        methods:
%s`, cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, cmd.TenantID, mode, cmd.WorkloadName, principalsBuilder.String(), methodsBuilder.String())
}
