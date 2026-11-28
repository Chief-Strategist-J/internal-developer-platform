/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DISASTER RECOVERY INDEX (V.3 role 13, X-05)

 1. Public Surface:
    - Exports only DisasterRecoveryModule and BuildModule constructor.
    - Internal services, adapters, and ports remain unexposed outside this package.
*/
package disasterrecovery

import "context"

type DRBridgeProvider interface {
	IsWorkloadFenced(ctx context.Context, tenantID, workloadID string) (bool, error)
	GetFencingEpoch(ctx context.Context, datasetID string) (int64, error)
}
