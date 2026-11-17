package cloudresource

import "time"

type ResourceType string

const (
	ResourceTypeWorkloadIdentity ResourceType = "iam_workload_identity"
	ResourceTypeDNSRecord        ResourceType = "dns_record"
	ResourceTypeCloudSQLBinding  ResourceType = "cloud_sql_binding"
)

type ResourceStatus string

const (
	ResourceStatusActive       ResourceStatus = "ACTIVE"
	ResourceStatusTornDown     ResourceStatus = "TORN_DOWN"
	ResourceStatusPendingSweep ResourceStatus = "PENDING_SWEEP"
	ResourceStatusOrphaned     ResourceStatus = "ORPHANED"
)

const (
	ConfigConnectorAPIVersionIAM = "iam.cnrm.cloud.google.com/v1beta1"
	ConfigConnectorAPIVersionDNS = "dns.cnrm.cloud.google.com/v1beta1"
	ConfigConnectorAPIVersionSQL = "sql.cnrm.cloud.google.com/v1beta1"

	KindIAMPolicyMember   = "IAMPolicyMember"
	KindIAMServiceAccount = "IAMServiceAccount"
	KindDNSRecordSet      = "DNSRecordSet"
	KindSQLDatabase       = "SQLDatabase"
	KindSQLUser           = "SQLUser"

	WorkloadIdentityUserRole = "roles/iam.workloadIdentityUser"
	WorkloadIdentityPoolFmt  = "%s.svc.id.goog[%s/%s]"
	WorkloadIdentityMember   = "serviceAccount:%s.svc.id.goog[%s/%s]"

	LabelRequestID   = "platform.gov/request-id"
	LabelOwner       = "platform.gov/owner"
	LabelCostCenter  = "platform.gov/cost-center"
	LabelWorkloadID  = "platform.gov/workload-id"
	LabelEnvironment = "platform.gov/environment"
	LabelManagedBy   = "app.kubernetes.io/managed-by"

	ManagedByPlatformValue = "argo-cd-platform"

	DNSTypeA     = "A"
	DNSTypeCNAME = "CNAME"
	DNSTypeTXT   = "TXT"

	SQLUserTypeCloudIAM = "CLOUD_IAM_SERVICE_ACCOUNT"
)

var PermittedIAMRoles = map[string]bool{
	"roles/cloudkms.cryptoKeyEncrypterDecrypter": true,
	"roles/secretmanager.secretAccessor":         true,
	"roles/storage.objectViewer":                 true,
	"roles/storage.objectCreator":                true,
	"roles/cloudsql.client":                      true,
	"roles/cloudsql.instanceUser":                true,
	"roles/monitoring.metricWriter":              true,
	"roles/logging.logWriter":                    true,
	"roles/cloudtrace.agent":                     true,
}

var ProhibitedIAMRoles = map[string]bool{
	"roles/owner":       true,
	"roles/editor":      true,
	"roles/resourcemanager.organizationAdmin": true,
	"roles/resourcemanager.folderAdmin":       true,
	"roles/iam.securityAdmin":                 true,
	"roles/iam.serviceAccountKeyAdmin":        true,
}

type ResourceMetadata struct {
	RequestID   string `json:"request_id"`
	Owner       string `json:"owner"`
	CostCenter  string `json:"cost_center"`
	WorkloadID  string `json:"workload_id"`
	Environment string `json:"environment"`
}

type WorkloadIdentityBindingSpec struct {
	GSAEmail     string           `json:"gsa_email"`
	KSAName      string           `json:"ksa_name"`
	Namespace    string           `json:"namespace"`
	ProjectID    string           `json:"project_id"`
	RoleBindings []string         `json:"role_bindings"`
	Metadata     ResourceMetadata `json:"metadata"`
}

type DNSRecordSpec struct {
	ZoneName   string           `json:"zone_name"`
	RecordName string           `json:"record_name"`
	RecordType string           `json:"record_type"`
	TTL        int              `json:"ttl"`
	TargetIPs  []string         `json:"target_ips"`
	TargetHost string           `json:"target_host"`
	Metadata   ResourceMetadata `json:"metadata"`
}

type CloudSQLBindingSpec struct {
	InstanceID         string           `json:"instance_id"`
	DatabaseName       string           `json:"database_name"`
	AppUser            string           `json:"app_user"`
	GSAEmail           string           `json:"gsa_email"`
	IAMAuthEnabled     bool             `json:"iam_auth_enabled"`
	DeletionProtection bool             `json:"deletion_protection"`
	Metadata           ResourceMetadata `json:"metadata"`
}

type ManifestResult struct {
	ResourceID   string           `json:"resource_id"`
	ResourceType ResourceType     `json:"resource_type"`
	ManifestYAML string           `json:"manifest_yaml"`
	Metadata     ResourceMetadata `json:"metadata"`
	CreatedAt    time.Time        `json:"created_at"`
}

type ResourceLedgerRecord struct {
	ResourceID         string           `json:"resource_id"`
	ResourceType       ResourceType     `json:"resource_type"`
	WorkloadID         string           `json:"workload_id"`
	Status             ResourceStatus   `json:"status"`
	DeletionProtection bool             `json:"deletion_protection"`
	Metadata           ResourceMetadata `json:"metadata"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

type LedgerReconciliationReport struct {
	TotalManaged        int      `json:"total_managed"`
	ActiveCount         int      `json:"active_count"`
	OrphanedCount       int      `json:"orphaned_count"`
	OrphanedResourceIDs []string `json:"orphaned_resource_ids"`
}
