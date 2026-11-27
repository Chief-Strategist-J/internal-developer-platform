/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME INDEX (V.3 role 13, X-05)

 1. Public Surface:
    - Exports only PlatformRegimeModule, BuildModule, DefaultBaselinePolicy, and RegimeBridgeProvider.
    - Internal services, adapters, and ports stay behind internal/.
*/
package platformregime

import "context"

type BaselinePolicyConfig struct {
	ID                   string
	Jurisdiction         string
	AuditRetentionDays   int
	DataResidencyRegions []string
	KMSKeyCustodyMode    string
	RTOMinutesTier0      int
	RPOMinutesTier0      int
	RTOMinutesTier1      int
	RPOMinutesTier1      int
	ClusterMode          string
	PrimaryRegionCount   int
	StandbyRegionCount   int
	TenantIsolationMode  string
	Status               string
}

type WorkloadComplianceEvalInput struct {
	Namespace           string
	AuditRetentionDays  int
	HasEncryptedSecrets bool
	KMSKeyURI           string
}

type WorkloadComplianceEvalViolation struct {
	Rule    string
	Message string
}

type RegimeBridgeProvider interface {
	EvaluateWorkloadCompliance(ctx context.Context, tenantID string, input WorkloadComplianceEvalInput) (bool, []WorkloadComplianceEvalViolation, error)
}

func DefaultBaselinePolicyConfig() BaselinePolicyConfig {
	return BaselinePolicyConfig{
		ID:                   "baseline-default",
		Jurisdiction:         "US_FEDRAMP_MODERATE",
		AuditRetentionDays:   365,
		DataResidencyRegions: []string{"us-central1", "us-east4"},
		KMSKeyCustodyMode:    "CMEK",
		RTOMinutesTier0:      15,
		RPOMinutesTier0:      0,
		RTOMinutesTier1:      60,
		RPOMinutesTier1:      15,
		ClusterMode:          "GKE_STANDARD",
		PrimaryRegionCount:   2,
		StandbyRegionCount:   1,
		TenantIsolationMode:  "INTERNAL_ONLY",
		Status:               "ACTIVE",
	}
}
