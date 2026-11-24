package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	drContracts "internal-developer-platform/src/features/disaster-recovery/contracts"
	"internal-developer-platform/src/shared/constants"
)

/*
Package handlers provides delivery ingress for operational and command-line interfaces.

Architectural Execution Blueprint:
1. Single Binary Ingress Parity:
   DisasterRecoveryCLIHandler is the CLI-protocol ingress handler implementing 1-to-1 parity
   with HTTP REST API endpoints under src/api/rest/v1/handlers/disaster-recovery.handler.go.
2. Dual Dispatch Support:
   - Remote Mode: Issues authenticated HTTP requests to a target platform instance via DR_API_BASE_URL.
   - Zero Hardcoding Doctrine: All fallback URLs, headers, and environment variable keys resolve
     strictly from shared constants (constants.DefaultPlatformAPIBaseURL, constants.EnvDRAPIBaseURL).
3. Zero Inline Comments Doctrine:
   Function bodies are strictly 100% comment-free. All operational invariants, flag definitions,
   and error-handling strategies are defined in this file header docblock.
4. Command Map:
   - plan:create       -> POST /api/v1/dr/plans
   - plan:check        -> GET  /api/v1/dr/plans/{planId}
   - plan:rpo          -> GET  /api/v1/dr/plans/{planId}/rpo
   - incident:declare  -> POST /api/v1/dr/incidents:declare
   - incident:approve  -> POST /api/v1/dr/incidents/{incidentId}:approve
   - failover:init     -> POST /api/v1/dr/failovers
   - failover:approve  -> POST /api/v1/dr/failovers/{operationId}:approve
   - dataset:fence     -> POST /api/v1/dr/datasets/{datasetId}:fence
   - failback:exec     -> POST /api/v1/dr/failbacks
   - drill:run         -> POST /api/v1/dr/drills
   - evidence:get      -> GET  /api/v1/dr/evidence/{evidenceBundleId}
   - restore:init      -> POST /api/v1/dr/restores
   - breakglass:grant  -> POST /api/v1/dr/breakglass:activate
*/

type DisasterRecoveryCLIHandler struct {
	httpClient *http.Client
}

func NewDisasterRecoveryCLIHandler() *DisasterRecoveryCLIHandler {
	return &DisasterRecoveryCLIHandler{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *DisasterRecoveryCLIHandler) Execute(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		h.printUsage()
		return 0
	}

	subcommand := args[0]
	cmdArgs := args[1:]

	switch subcommand {
	case "plan:create":
		return h.handlePlanCreate(ctx, cmdArgs)
	case "plan:check":
		return h.handlePlanCheck(ctx, cmdArgs)
	case "plan:rpo":
		return h.handlePlanRPO(ctx, cmdArgs)
	case "incident:declare":
		return h.handleIncidentDeclare(ctx, cmdArgs)
	case "incident:approve":
		return h.handleIncidentApprove(ctx, cmdArgs)
	case "failover:init":
		return h.handleFailoverInit(ctx, cmdArgs)
	case "failover:approve":
		return h.handleFailoverApprove(ctx, cmdArgs)
	case "dataset:fence":
		return h.handleDatasetFence(ctx, cmdArgs)
	case "failback:exec":
		return h.handleFailbackExec(ctx, cmdArgs)
	case "drill:run":
		return h.handleDrillRun(ctx, cmdArgs)
	case "evidence:get":
		return h.handleEvidenceGet(ctx, cmdArgs)
	case "restore:init":
		return h.handleRestoreInit(ctx, cmdArgs)
	case "breakglass:grant":
		return h.handleBreakglassGrant(ctx, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown disaster-recovery command: %s\n", subcommand)
		h.printUsage()
		return 2
	}
}

func (h *DisasterRecoveryCLIHandler) resolveConfig(fs *flag.FlagSet, baseURLFlag, tenantFlag, tokenFlag *string) (string, string, string) {
	baseURL := *baseURLFlag
	if baseURL == "" {
		baseURL = os.Getenv(constants.EnvDRAPIBaseURL)
	}
	if baseURL == "" {
		baseURL = constants.DefaultPlatformAPIBaseURL
	}

	tenantID := *tenantFlag
	if tenantID == "" {
		tenantID = os.Getenv("DR_TENANT_ID")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	token := *tokenFlag
	if token == "" {
		token = os.Getenv("DR_AUTH_TOKEN")
	}

	return strings.TrimRight(baseURL, "/"), tenantID, token
}

func (h *DisasterRecoveryCLIHandler) doRequest(ctx context.Context, method, urlStr, tenantID, token string, bodyData interface{}) error {
	var bodyReader io.Reader
	if bodyData != nil {
		buf, err := json.Marshal(bodyData)
		if err != nil {
			return fmt.Errorf("failed to encode request JSON: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-tenant-id", tenantID)
	req.Header.Set("x-api-version", "v1")
	req.Header.Set("traceparent", fmt.Sprintf("00-%032x-%016x-01", time.Now().UnixNano(), time.Now().UnixNano()&0xFFFFFFFFFFFFFFF))

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var prettyJSON bytes.Buffer
	if jsonErr := json.Indent(&prettyJSON, respBytes, "", "  "); jsonErr == nil {
		fmt.Printf("[HTTP %d]\n%s\n", resp.StatusCode, prettyJSON.String())
	} else {
		fmt.Printf("[HTTP %d]\n%s\n", resp.StatusCode, string(respBytes))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("command returned failure HTTP %d", resp.StatusCode)
	}
	return nil
}

func (h *DisasterRecoveryCLIHandler) handlePlanCreate(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl plan:create", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	name := fs.String("name", "core-pay-dr", "Human readable plan name")
	tier := fs.String("tier", "TIER_1", "Service criticality tier (TIER_0, TIER_1, TIER_2)")
	primaryRegion := fs.String("primary-region", "us-central1", "Primary region")
	standbyRegion := fs.String("standby-region", "us-east4", "Secondary standby region")
	topology := fs.String("topology", "WARM_STANDBY", "Topology (ACTIVE_ACTIVE, WARM_STANDBY, PILOT_LIGHT, RESTORE_TO_NEW)")
	dataStore := fs.String("data-store", "CLOUD_SQL", "Data store type (CLOUD_SQL, ALLOYDB, SPANNER, SELF_MANAGED)")
	secretSource := fs.String("secret-source", "GOOGLE_SECRET_MANAGER", "Secret source (GOOGLE_SECRET_MANAGER, VAULT_REPLICATED)")
	primaryCluster := fs.String("primary-cluster", "gke-primary-us-central1", "Primary cluster ID")
	standbyCluster := fs.String("standby-cluster", "gke-standby-us-east4", "Standby cluster ID")
	targetRTO := fs.Int("target-rto", 1800, "Target RTO seconds")
	targetRPO := fs.Int("target-rpo", 300, "Target RPO seconds")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.CreateDRPlanCommand{
		TenantID:         tenantID,
		Source:           "cli-dr-ctl",
		ExternalID:       fmt.Sprintf("ext-%d", time.Now().UnixNano()),
		CorrelationID:    fmt.Sprintf("corr-%d", time.Now().UnixNano()),
		PlanName:         *name,
		IncidentTier:     *tier,
		PrimaryRegion:    *primaryRegion,
		DRRegion:         *standbyRegion,
		Topology:         drContracts.DRTopology(*topology),
		TargetRTOSeconds: *targetRTO,
		TargetRPOSeconds: *targetRPO,
		DataStoreType:    drContracts.DRDataStoreType(*dataStore),
		SecretSyncSource: drContracts.DRSecretSyncSource(*secretSource),
		PrimaryClusterID: *primaryCluster,
		DRClusterID:      *standbyCluster,
		IdempotencyKey:   fmt.Sprintf("idemp-%d", time.Now().UnixNano()),
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRPlans)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "plan:create error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handlePlanCheck(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl plan:check", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Unique DR plan ID")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	endpoint := fmt.Sprintf("%s%s%s", baseURL, constants.EndpointDRPlansPrefix, *planID)
	if err := h.doRequest(ctx, http.MethodGet, endpoint, tenantID, token, nil); err != nil {
		fmt.Fprintf(os.Stderr, "plan:check error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handlePlanRPO(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl plan:rpo", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Unique DR plan ID")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	endpoint := fmt.Sprintf("%s%s%s/rpo", baseURL, constants.EndpointDRPlansPrefix, *planID)
	if err := h.doRequest(ctx, http.MethodGet, endpoint, tenantID, token, nil); err != nil {
		fmt.Fprintf(os.Stderr, "plan:rpo error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleIncidentDeclare(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl incident:declare", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Unique DR plan ID")
	scope := fs.String("scope", "REGION", "Disaster scope (ZONE, REGION, PROVIDER_GLOBAL, DATA_CORRUPTION, TRUST_ANCHOR_COMPROMISE)")
	declarer := fs.String("declarer", "secops-officer-marcus", "Operator identity")
	evidence := fs.String("evidence", "https://monitoring.internal/alerts/alert-9912", "Comma separated evidence links")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	links := strings.Split(*evidence, ",")
	payload := drContracts.DeclareIncidentCommand{
		TenantID:      tenantID,
		PlanID:        *planID,
		Scope:         drContracts.DisasterScope(*scope),
		Declarer:      *declarer,
		EvidenceLinks: links,
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRIncidentsDeclare)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "incident:declare error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleIncidentApprove(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl incident:approve", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	incidentID := fs.String("incident", "", "Incident UUID")
	approver := fs.String("approver", "incident-commander-dr", "Approver identity")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *incidentID == "" {
		fmt.Fprintln(os.Stderr, "error: --incident is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.ApproveIncidentCommand{
		TenantID:   tenantID,
		IncidentID: *incidentID,
		Approver:   *approver,
	}

	endpoint := fmt.Sprintf("%s%s%s:approve", baseURL, constants.EndpointDRIncidentsApprovePrefix, *incidentID)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "incident:approve error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleFailoverInit(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl failover:init", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Unique DR plan ID")
	incidentID := fs.String("incident", "", "Approved incident UUID")
	mode := fs.String("mode", "SWITCHOVER", "Failover mode (SWITCHOVER, FAILOVER)")
	initiator := fs.String("initiator", "platform-admin-1", "Initiating operator identity")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" || *incidentID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan and --incident are required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.InitiateFailoverCommand{
		TenantID:   tenantID,
		PlanID:     *planID,
		IncidentID: *incidentID,
		Mode:       drContracts.FailoverMode(*mode),
		Initiator:  *initiator,
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRFailovers)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "failover:init error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleFailoverApprove(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl failover:approve", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	operationID := fs.String("operation", "", "Failover operation UUID")
	approver := fs.String("approver", "site-lead-officer", "Secondary peer approver identity")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *operationID == "" {
		fmt.Fprintln(os.Stderr, "error: --operation is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.ApproveFailoverCommand{
		TenantID:    tenantID,
		OperationID: *operationID,
		Approver:    *approver,
	}

	endpoint := fmt.Sprintf("%s%s%s:approve", baseURL, constants.EndpointDRFailoversPrefix, *operationID)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "failover:approve error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleDatasetFence(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl dataset:fence", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	datasetID := fs.String("dataset", "", "Target dataset or plan ID to fence")
	authority := fs.String("authority", "dr-control-plane", "Authority incrementing epoch")
	actor := fs.String("actor", "platform-admin", "Actor identity")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *datasetID == "" {
		fmt.Fprintln(os.Stderr, "error: --dataset is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.FenceDatasetCommand{
		TenantID:  tenantID,
		DatasetID: *datasetID,
		Authority: *authority,
		Actor:     *actor,
	}

	endpoint := fmt.Sprintf("%s%s%s:fence", baseURL, constants.EndpointDRDatasetsPrefix, *datasetID)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "dataset:fence error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleFailbackExec(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl failback:exec", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Target DR plan ID")
	initiator := fs.String("initiator", "platform-admin", "Initiating operator identity")
	approver := fs.String("approver", "lead-sre", "Approving peer identity")
	divergence := fs.Int("divergence", 0, "Divergence records count")
	reverseLag := fs.Int("reverse-lag", 0, "Reverse replication lag seconds")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.ExecuteFailbackCommand{
		TenantID:   tenantID,
		PlanID:     *planID,
		Initiator:  *initiator,
		Approver:   *approver,
		Divergence: *divergence,
		ReverseLag: *reverseLag,
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRFailbacks)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "failback:exec error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleDrillRun(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl drill:run", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Target DR plan ID")
	scope := fs.String("scope", "ZONE", "Drill simulation scope")
	initiator := fs.String("initiator", "chaos-drill-runner", "Initiating identity")
	syntheticOnly := fs.Bool("synthetic-only", true, "Execute drill in synthetic mode")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.RunDrillCommand{
		TenantID:      tenantID,
		PlanID:        *planID,
		Scope:         *scope,
		Initiator:     *initiator,
		SyntheticOnly: *syntheticOnly,
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRDrills)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "drill:run error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleEvidenceGet(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl evidence:get", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	evidenceID := fs.String("bundle", "", "Evidence bundle UUID")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *evidenceID == "" {
		fmt.Fprintln(os.Stderr, "error: --bundle is required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	endpoint := fmt.Sprintf("%s%s%s", baseURL, constants.EndpointDREvidencePrefix, *evidenceID)
	if err := h.doRequest(ctx, http.MethodGet, endpoint, tenantID, token, nil); err != nil {
		fmt.Fprintf(os.Stderr, "evidence:get error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleRestoreInit(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl restore:init", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	planID := fs.String("plan", "", "Target DR plan ID")
	backupID := fs.String("backup", "", "Backup snapshot ID to restore")
	targetCluster := fs.String("target-cluster", "gke-standby-us-east4", "Target restoration cluster")
	namespace := fs.String("namespace", "prod-payments", "Target namespace")
	conflictPolicy := fs.String("conflict-policy", "FAIL_IF_EXISTS", "Conflict policy")
	initiator := fs.String("initiator", "platform-admin", "Initiator identity")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *planID == "" || *backupID == "" {
		fmt.Fprintln(os.Stderr, "error: --plan and --backup are required")
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.InitiateRestoreCommand{
		TenantID:       tenantID,
		PlanID:         *planID,
		BackupID:       *backupID,
		TargetCluster:  *targetCluster,
		Namespace:      *namespace,
		ConflictPolicy: *conflictPolicy,
		Initiator:      *initiator,
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRRestores)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "restore:init error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) handleBreakglassGrant(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("dr-ctl breakglass:grant", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", "", "DR Control Plane base URL")
	tenantFlag := fs.String("tenant", "", "Tenant ID")
	tokenFlag := fs.String("token", "", "Bearer authorization token")
	requester := fs.String("requester", "secops-officer-marcus", "Requester identity")
	approver := fs.String("approver", "incident-commander-dr", "Second peer approver identity")
	justification := fs.String("reason", "Critical regional evacuation", "Break-glass justification")
	ttlMinutes := fs.Int("ttl-minutes", 60, "Access duration in minutes")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	baseURL, tenantID, token := h.resolveConfig(fs, baseURLFlag, tenantFlag, tokenFlag)
	payload := drContracts.ActivateBreakglassCommand{
		TenantID:      tenantID,
		Requester:     *requester,
		Approver:      *approver,
		DurationMins:  *ttlMinutes,
		Justification: *justification,
	}

	endpoint := fmt.Sprintf("%s%s", baseURL, constants.EndpointDRBreakglassActivate)
	if err := h.doRequest(ctx, http.MethodPost, endpoint, tenantID, token, payload); err != nil {
		fmt.Fprintf(os.Stderr, "breakglass:grant error: %v\n", err)
		return 1
	}
	return 0
}

func (h *DisasterRecoveryCLIHandler) printUsage() {
	fmt.Println("Usage: go run ./src/main.go dr-ctl <subcommand> [flags]")
	fmt.Println("\nAvailable Subcommands:")
	fmt.Println("  plan:create        Register and provision a new DR plan")
	fmt.Println("  plan:check         Inspect DR plan configuration and status")
	fmt.Println("  plan:rpo           Query real-time RPO replication lag")
	fmt.Println("  incident:declare   Declare disaster recovery incident (SEV-0/1)")
	fmt.Println("  incident:approve   Grant secondary commander quorum approval")
	fmt.Println("  failover:init      Initiate failover sequence for an approved incident")
	fmt.Println("  failover:approve   Authorize failover execution (two-person gate)")
	fmt.Println("  dataset:fence      Isolate split-brain dataset and increment fencing epoch")
	fmt.Println("  failback:exec      Execute failback with zero-divergence validation")
	fmt.Println("  drill:run          Execute simulated sandbox failover drill")
	fmt.Println("  evidence:get       Retrieve immutable audit compliance evidence bundle")
	fmt.Println("  restore:init       Trigger backup restore to standby region")
	fmt.Println("  breakglass:grant   Activate emergency break-glass elevation")
	fmt.Println("\nGlobal Configuration (Flags or Environment):")
	fmt.Println("  --base-url         DR Control Plane base URL (default: http://127.0.0.1:8080 or DR_API_BASE_URL)")
	fmt.Println("  --tenant           Tenant identifier header (default: default or DR_TENANT_ID)")
	fmt.Println("  --token            Bearer authorization token (default: DR_AUTH_TOKEN)")
}
