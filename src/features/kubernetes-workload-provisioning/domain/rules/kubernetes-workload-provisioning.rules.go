package rules

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
	ImagePullPolicy string
	AuditLogging    contracts.AuditLoggingConfig
}

func resolveAuditDefaults(cmd contracts.WorkloadProvisionCommand) contracts.AuditLoggingConfig {
	auditCfg := cmd.AuditLogging
	if auditCfg.DestinationBucket == "" && !auditCfg.LockedRetention && auditCfg.RetentionDays == 0 {
		bucket := fmt.Sprintf("%s-%s-audit-logs", cmd.TenantID, cmd.Namespace)
		if cmd.TenantID == "" {
			bucket = fmt.Sprintf("%s-audit-logs", cmd.Namespace)
		}
		return contracts.AuditLoggingConfig{
			EnableDataAccessLogs: true,
			DestinationBucket:    bucket,
			RetentionDays:        400,
			LockedRetention:      true,
			OffNodeLogCollection: true,
			PIIScrubbingEnabled:  true,
		}
	}
	return auditCfg
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

	pullPolicy := cmd.ImagePullPolicy
	if pullPolicy == "" {
		if strings.Contains(cmd.Image, "@sha256:") {
			pullPolicy = "IfNotPresent"
		} else if strings.HasSuffix(cmd.Image, ":latest") {
			pullPolicy = "Always"
		} else {
			pullPolicy = "IfNotPresent"
		}
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
		ImagePullPolicy: pullPolicy,
		AuditLogging:    resolveAuditDefaults(cmd),
	}
}

func IsApprovedRegistry(image string) bool {
	parts := strings.Split(image, "/")
	if len(parts) < 2 {
		return false
	}
	domain := parts[0]
	if domain == "repo" || domain == "ghcr.io" || domain == "quay.io" {
		return true
	}
	if domain == "gcr.io" || strings.HasSuffix(domain, ".gcr.io") {
		return true
	}
	if domain == "pkg.dev" || strings.HasSuffix(domain, ".pkg.dev") {
		return true
	}
	if strings.HasSuffix(domain, ".local") || strings.HasSuffix(domain, ".internal") {
		return true
	}
	return false
}

func ValidateImageSupplyChain(cmd contracts.WorkloadProvisionCommand) (bool, []contracts.AdmissionViolation) {
	violations := make([]contracts.AdmissionViolation, 0)
	critical := false
	isProd := strings.Contains(cmd.Namespace, "prod")

	if !IsApprovedRegistry(cmd.Image) {
		critical = true
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "IMG-REG-001",
			Message: fmt.Sprintf("image registry in %q is not in approved registry floor", cmd.Image),
		})
	}

	hasDigest := strings.Contains(cmd.Image, "@sha256:")
	isLatest := strings.HasSuffix(cmd.Image, ":latest")

	if isProd {
		if isLatest {
			critical = true
			violations = append(violations, contracts.AdmissionViolation{
				Rule:    "IMG-TAG-001",
				Message: "mutable image tag :latest is strictly prohibited in production",
			})
		}
		if !hasDigest {
			if cmd.BreakglassJustification != "" {
				violations = append(violations, contracts.AdmissionViolation{
					Rule:    "IMG-BRK-001",
					Message: "breakglass emergency deploy admitted without immutable digest: " + cmd.BreakglassJustification,
				})
			} else {
				critical = true
				violations = append(violations, contracts.AdmissionViolation{
					Rule:    "IMG-DIGEST-001",
					Message: "production workloads strictly mandate immutable @sha256 digest",
				})
			}
		}
	} else if isLatest {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "IMG-001",
			Message: "Warning: Mutable image tag :latest detected",
		})
	}

	return !critical, violations
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

	imgOk, imgViolations := ValidateImageSupplyChain(cmd)
	if !imgOk {
		critical = true
	}
	violations = append(violations, imgViolations...)

	secOk, secViolations := ValidateSecretVolumeMounts(cmd.SecretMounts)
	if !secOk {
		critical = true
	}
	violations = append(violations, secViolations...)

	kmsOk, kmsViolations := ValidateKMSKeyCustody(cmd.TenantID, cmd.KMSKeyURI, cmd.EncryptedSecrets)
	if !kmsOk {
		critical = true
	}
	violations = append(violations, kmsViolations...)

	meshOk, meshViolations := ValidateMeshSecurity(cmd.Namespace, cmd.MeshSecurity)
	if !meshOk {
		critical = true
	}
	violations = append(violations, meshViolations...)

	crdOk, crdViolations := ValidateMeshCRDCompatibility(cmd.UnsupportedMeshResources)
	if !crdOk {
		critical = true
	}
	violations = append(violations, crdViolations...)

	audOk, audViolations := ValidateAuditLogging(cmd.Namespace, mutations.AuditLogging)
	if !audOk {
		critical = true
	}
	violations = append(violations, audViolations...)

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

func CalculateSurgePods(replicas int, strategy contracts.DeploymentStrategy) int {
	if replicas <= 0 {
		return 1
	}
	if strategy == contracts.StrategyRecreate {
		return 0
	}
	surge := (replicas + 3) / 4
	if surge < 1 {
		surge = 1
	}
	return surge
}

func EvaluateResourceQuotaSurgeHeadroom(quota contracts.ResourceQuotaSpec, requestedCPU string, requestedMemory string, replicas int, strategy contracts.DeploymentStrategy) contracts.QuotaCheckResult {
	if replicas <= 0 {
		replicas = 1
	}
	surgePods := CalculateSurgePods(replicas, strategy)
	totalPods := int64(replicas + surgePods)

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
	totalReqCPU := reqCPUMilli * totalPods

	reqMemMB, err := ParseMegabytes(requestedMemory)
	if err != nil {
		reqMemMB = 256
	}
	totalReqMem := reqMemMB * totalPods

	if totalReqCPU > availCPU {
		return contracts.QuotaCheckResult{
			SufficientHeadroom: false,
			AvailableCPU:       fmt.Sprintf("%dm", availCPU),
			AvailableMemory:    fmt.Sprintf("%dMi", availMem),
			ViolationMessage:   fmt.Sprintf("quota exceeded for surge: requested cpu %dm (base %d + surge %d pods) exceeds available %dm", totalReqCPU, replicas, surgePods, availCPU),
		}
	}

	if totalReqMem > availMem {
		return contracts.QuotaCheckResult{
			SufficientHeadroom: false,
			AvailableCPU:       fmt.Sprintf("%dm", availCPU),
			AvailableMemory:    fmt.Sprintf("%dMi", availMem),
			ViolationMessage:   fmt.Sprintf("quota exceeded for surge: requested memory %dMi (base %d + surge %d pods) exceeds available %dMi", totalReqMem, replicas, surgePods, availMem),
		}
	}

	return contracts.QuotaCheckResult{
		SufficientHeadroom: true,
		AvailableCPU:       fmt.Sprintf("%dm", availCPU),
		AvailableMemory:    fmt.Sprintf("%dMi", availMem),
	}
}

func EvaluateResourceQuotaHeadroom(quota contracts.ResourceQuotaSpec, requestedCPU string, requestedMemory string, replicas int) contracts.QuotaCheckResult {
	return EvaluateResourceQuotaSurgeHeadroom(quota, requestedCPU, requestedMemory, replicas, contracts.StrategyCanary)
}

func ValidateSpecApproval(cmd contracts.WorkloadProvisionCommand) error {
	specHash := contracts.ComputeWorkloadSpecHash(cmd)
	if cmd.ApprovedSpecHash != "" && cmd.ApprovedSpecHash != specHash {
		return errors.NewConflictError(fmt.Sprintf(
			"spec edited during approval: approved hash %s does not match current spec hash %s",
			cmd.ApprovedSpecHash, specHash,
		))
	}
	isProduction := strings.Contains(cmd.Namespace, "prod") || strings.EqualFold(cmd.TenantID, "prod")
	if isProduction && cmd.ApprovedSpecHash == "" {
		return errors.NewForbiddenError("production workload changes require explicit approval bound to spec hash")
	}
	return nil
}

const ArgoCDAppTemplate = `apiVersion: argoproj.io/v1alpha1
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
    - group: networking.istio.io
      kind: VirtualService
      jsonPointers:
        - /spec/http/0/route
    - group: argoproj.io
      kind: Rollout
      jsonPointers:
        - /status
`

func resolveFallback(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func resolveAppPath(explicitPath, namespace, workloadName string) string {
	if explicitPath != "" {
		return explicitPath
	}
	return fmt.Sprintf("workloads/%s/%s", namespace, workloadName)
}

func GenerateArgoCDApplicationYAML(cmd contracts.WorkloadProvisionCommand, appConfig contracts.GitOpsAppConfig) string {
	project := resolveFallback(appConfig.ProjectName, "default")
	revision := resolveFallback(appConfig.TargetRevision, "HEAD")
	server := resolveFallback(appConfig.ServerURL, "https://kubernetes.default.svc")
	repo := resolveFallback(appConfig.RepoURL, "https://github.com/Chief-Strategist-J/platform-gitops-manifests.git")
	path := resolveAppPath(appConfig.Path, cmd.Namespace, cmd.WorkloadName)
	return fmt.Sprintf(ArgoCDAppTemplate, cmd.WorkloadName, cmd.TenantID, project, repo, revision, path, server, cmd.Namespace)
}

func EvaluateDefaultRolloutConfig(cmd contracts.WorkloadProvisionCommand) contracts.RolloutConfig {
	strategy := cmd.Strategy
	if strategy == "" {
		strategy = contracts.StrategyCanary
	}
	errorDeltaQuery := fmt.Sprintf(`(sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-canary",response_code=~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-canary"}[1m]))) - (sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-stable",response_code=~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination",destination_workload="%s-stable"}[1m])))`,
		cmd.WorkloadName, cmd.WorkloadName, cmd.WorkloadName, cmd.WorkloadName)
	latencyP99Query := fmt.Sprintf(`histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{reporter="destination",destination_workload="%s-canary"}[1m])) by (le))`,
		cmd.WorkloadName)

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
				Name:                  "success-rate",
				Interval:              "30s",
				Count:                 5,
				SuccessCondition:      "result[0] <= 0.001",
				FailureLimit:          1,
				InconclusiveLimit:     0,
				ConsecutiveErrorLimit: 1,
				Query:                 errorDeltaQuery,
			},
			{
				Name:                  "latency-p99",
				Interval:              "30s",
				Count:                 5,
				SuccessCondition:      "result[0] <= 500",
				FailureLimit:          1,
				InconclusiveLimit:     0,
				ConsecutiveErrorLimit: 1,
				Query:                 latencyP99Query,
			},
		},
	}
}

func GenerateAnalysisTemplateYAML(cmd contracts.WorkloadProvisionCommand, metrics []contracts.AnalysisMetric) string {
	var metricsBuilder strings.Builder
	for _, m := range metrics {
		count := m.Count
		if count <= 0 {
			count = 5
		}
		consecLimit := m.ConsecutiveErrorLimit
		if consecLimit <= 0 {
			consecLimit = 1
		}
		failLimit := m.FailureLimit
		if failLimit <= 0 {
			failLimit = 1
		}
		metricsBuilder.WriteString(fmt.Sprintf(`    - name: %s
      interval: %s
      count: %d
      successCondition: %s
      failureLimit: %d
      inconclusiveLimit: %d
      consecutiveErrorLimit: %d
      provider:
        prometheus:
          address: http://prometheus-k8s.monitoring.svc:9090
          query: %s
`, m.Name, m.Interval, count, m.SuccessCondition, failLimit, m.InconclusiveLimit, consecLimit, m.Query))
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

const RolloutTemplate = `apiVersion: argoproj.io/v1alpha1
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
        imagePullPolicy: %s
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
`

func buildRolloutSteps(steps []contracts.CanaryStep) string {
	var sb strings.Builder
	for _, step := range steps {
		sb.WriteString(fmt.Sprintf("        - setWeight: %d\n", step.SetWeight))
		if step.PauseDurationSeconds > 0 {
			sb.WriteString(fmt.Sprintf("        - pause: { duration: %ds }\n", step.PauseDurationSeconds))
		}
	}
	return sb.String()
}

func GenerateArgoRolloutYAML(cmd contracts.WorkloadProvisionCommand, mutations WorkloadMutations, rolloutCfg contracts.RolloutConfig) string {
	steps := buildRolloutSteps(rolloutCfg.CanarySteps)
	sc := mutations.SecurityContext
	pb := mutations.Probes
	return fmt.Sprintf(RolloutTemplate,
		cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, cmd.TenantID,
		cmd.Replicas, cmd.WorkloadName, cmd.WorkloadName, cmd.WorkloadName,
		steps, cmd.WorkloadName, cmd.TenantID,
		sc.RunAsNonRoot, sc.RunAsUser, sc.RunAsGroup, sc.SeccompProfileType,
		cmd.WorkloadName, cmd.Image, mutations.ImagePullPolicy,
		sc.AllowPrivilegeEscalation, sc.ReadOnlyRootFilesystem,
		mutations.CPURequest, mutations.MemoryRequest, mutations.CPULimit, mutations.MemoryLimit,
		pb.StartupPath, pb.Port, pb.LivenessPath, pb.Port, pb.ReadinessPath, pb.Port,
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

func ValidateSecretVolumeMounts(mounts []contracts.SecretVolumeMount) (bool, []contracts.AdmissionViolation) {
	violations := make([]contracts.AdmissionViolation, 0)
	critical := false
	for _, m := range mounts {
		if strings.TrimSpace(m.SubPath) != "" {
			critical = true
			violations = append(violations, contracts.AdmissionViolation{
				Rule:    "SEC-SUBPATH-001",
				Message: fmt.Sprintf("subPath %q on secret mount %q is prohibited; subPath mounts break atomic rotation (V-15)", m.SubPath, m.SecretName),
			})
		}
	}
	return !critical, violations
}

func ValidateKMSKeyCustody(tenantID, keyURI string, encryptedSecrets map[string]string) (bool, []contracts.AdmissionViolation) {
	violations := make([]contracts.AdmissionViolation, 0)
	if len(encryptedSecrets) > 0 && strings.TrimSpace(keyURI) == "" {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "SEC-KMS-001",
			Message: "KMS key URI is required when encrypted secrets are configured (V-34)",
		})
		return false, violations
	}
	if strings.TrimSpace(keyURI) == "" {
		return true, violations
	}
	prefix := fmt.Sprintf("projects/%s/", strings.TrimSpace(tenantID))
	if strings.Contains(keyURI, prefix) {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "SEC-KMS-002",
			Message: "KMS key must reside in a dedicated security project, not workload tenant project (V-34)",
		})
		return false, violations
	}
	return true, violations
}

func EvaluateCertificateExpiry(expiresAt time.Time, renewBefore time.Duration) contracts.CertificateExpiryStatus {
	now := time.Now().UTC()
	diff := expiresAt.Sub(now)
	daysRemaining := int(diff.Hours() / 24)
	if diff <= 0 {
		return contracts.CertificateExpiryStatus{
			NeedsRenewal:   true,
			DaysRemaining:  daysRemaining,
			AlertSeverity:  "CRITICAL_EXPIRED",
			ExpirationTime: expiresAt,
		}
	}
	if diff <= renewBefore {
		return contracts.CertificateExpiryStatus{
			NeedsRenewal:   true,
			DaysRemaining:  daysRemaining,
			AlertSeverity:  "WARNING_EXPIRING_SOON",
			ExpirationTime: expiresAt,
		}
	}
	return contracts.CertificateExpiryStatus{
		NeedsRenewal:   false,
		DaysRemaining:  daysRemaining,
		AlertSeverity:  "HEALTHY",
		ExpirationTime: expiresAt,
	}
}

func ValidateMeshSecurity(namespace string, secCfg contracts.MeshSecurityConfig) (bool, []contracts.AdmissionViolation) {
	violations := make([]contracts.AdmissionViolation, 0)
	if strings.Contains(namespace, "prod") {
		mode := strings.ToUpper(strings.TrimSpace(secCfg.MtlsMode))
		if mode != "" && mode != "STRICT" {
			violations = append(violations, contracts.AdmissionViolation{
				Rule:    "MESH-MTLS-001",
				Message: "production namespaces mandate mTLS mode STRICT (V-30)",
			})
			return false, violations
		}
	}
	return true, violations
}

func ValidateMeshCRDCompatibility(resources []string) (bool, []contracts.AdmissionViolation) {
	violations := make([]contracts.AdmissionViolation, 0)
	for _, r := range resources {
		clean := strings.ToLower(strings.TrimSpace(r))
		if clean == "envoyfilter" || clean == "wasmplugin" || clean == "istiod" || clean == "in-cluster-istio" {
			violations = append(violations, contracts.AdmissionViolation{
				Rule:    "MESH-OSS-001",
				Message: fmt.Sprintf("unsupported mesh resource %q is prohibited; Cloud Service Mesh managed mode strictly prohibits in-cluster mesh control planes (V-30)", r),
			})
			return false, violations
		}
	}
	return true, violations
}

func GeneratePeerAuthenticationYAML(namespace, workloadName, mode string) string {
	effectiveMode := strings.ToUpper(strings.TrimSpace(mode))
	if effectiveMode == "" {
		effectiveMode = "STRICT"
	}
	return fmt.Sprintf(`apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: %s-peer-authn
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/managed-by: internal-developer-platform
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  mtls:
    mode: %s`, workloadName, namespace, workloadName, workloadName, effectiveMode)
}

func GenerateServiceEntryYAML(cmd contracts.WorkloadProvisionCommand) string {
	if len(cmd.EgressRules) == 0 {
		return ""
	}
	var hosts strings.Builder
	var ports strings.Builder
	for _, r := range cmd.EgressRules {
		hosts.WriteString(fmt.Sprintf("  - %s\n", r.Host))
		proto := r.Protocol
		if proto == "" {
			proto = "HTTPS"
		}
		ports.WriteString(fmt.Sprintf("  - number: %d\n    name: %s-%d\n    protocol: %s\n", r.Port, strings.ToLower(proto), r.Port, proto))
	}
	return fmt.Sprintf(`apiVersion: networking.istio.io/v1beta1
kind: ServiceEntry
metadata:
  name: %s-egress-allowlist
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/managed-by: internal-developer-platform
spec:
  hosts:
%s  ports:
%s  resolution: DNS
  location: MESH_EXTERNAL`, cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, hosts.String(), ports.String())
}

func ValidateAuditLogging(namespace string, cfg contracts.AuditLoggingConfig) (bool, []contracts.AdmissionViolation) {
	violations := make([]contracts.AdmissionViolation, 0)
	if !strings.Contains(namespace, "prod") {
		return true, violations
	}
	if !cfg.EnableDataAccessLogs {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "AUD-DATA-001",
			Message: "production namespaces mandate Data Access audit logs enabled (V-32)",
		})
	}
	if cfg.DestinationBucket == "" || cfg.DestinationBucket == "_Default" {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "AUD-SINK-001",
			Message: "audit logs must route to a dedicated user-defined bucket, not _Default 30-day retention bucket (V-32)",
		})
	}
	if cfg.RetentionDays < 365 || !cfg.LockedRetention {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "AUD-LOCK-001",
			Message: "production audit logs require locked retention >= 365 days (V-32)",
		})
	}
	if !cfg.OffNodeLogCollection || !cfg.PIIScrubbingEnabled {
		violations = append(violations, contracts.AdmissionViolation{
			Rule:    "AUD-COLL-001",
			Message: "production logging requires off-node shipment and PII scrubbing (V-23)",
		})
	}
	return len(violations) == 0, violations
}

func GenerateLogSinkYAML(cmd contracts.WorkloadProvisionCommand) string {
	cfg := cmd.AuditLogging
	if cfg.DestinationBucket == "" {
		return ""
	}
	return fmt.Sprintf(`apiVersion: logging.cnrm.cloud.google.com/v1beta1
kind: LoggingLogSink
metadata:
  name: %s-audit-sink
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/managed-by: internal-developer-platform
spec:
  destination: storage.googleapis.com/%s
  filter: logName:"cloudaudit.googleapis.com%%2Fdata_access" OR resource.type="k8s_container"
  uniqueWriterIdentity: true`, cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, cfg.DestinationBucket)
}

func GenerateLoggingBucketYAML(cmd contracts.WorkloadProvisionCommand) string {
	cfg := cmd.AuditLogging
	if cfg.DestinationBucket == "" {
		return ""
	}
	retentionSeconds := int64(cfg.RetentionDays) * 86400
	if retentionSeconds <= 0 {
		retentionSeconds = 365 * 86400
	}
	kmsKey := cfg.KMSKeyURI
	if kmsKey == "" {
		kmsKey = cmd.KMSKeyURI
	}
	return fmt.Sprintf(`apiVersion: storage.cnrm.cloud.google.com/v1beta1
kind: StorageBucket
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/managed-by: internal-developer-platform
spec:
  lifecycleRule:
  - action:
      type: Delete
    condition:
      age: %d
  retentionPolicy:
    isLocked: %t
    retentionPeriod: %d
  encryption:
    defaultKmsKeyName: "%s"`, cfg.DestinationBucket, cmd.Namespace, cmd.WorkloadName, cfg.RetentionDays, cfg.LockedRetention, retentionSeconds, kmsKey)
}

func ValidateEphemeralDebugContainer(image string, approvedRegistries []string, isProduction bool) error {
	if image == "" {
		return errors.NewValidationError("debug image is required")
	}
	if isProduction && !strings.Contains(image, "@sha256:") {
		return errors.NewValidationError("V-26 violation: production ephemeral debug containers must reference immutable digest (@sha256:)")
	}
	if len(approvedRegistries) > 0 {
		approved := false
		for _, reg := range approvedRegistries {
			if strings.HasPrefix(image, reg) {
				approved = true
				break
			}
		}
		if !approved {
			return errors.NewValidationError("V-26 violation: debug image registry not permitted in cluster")
		}
	}
	return nil
}

func GenerateJITRoleBindingYAML(grantID, namespace, subjectID, role, approvedBy, reason, expiresAt string) string {
	return fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: jit-grant-%s
  namespace: %s
  annotations:
    security.internal.idp/expires-at: "%s"
    security.internal.idp/approved-by: "%s"
    security.internal.idp/reason: "%s"
subjects:
- kind: User
  name: %s
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: %s
  apiGroup: rbac.authorization.k8s.io`, grantID, namespace, expiresAt, approvedBy, reason, subjectID, role)
}




