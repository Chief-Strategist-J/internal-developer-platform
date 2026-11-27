/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY AS CODE INDEX (V.3 role 13, X-05)

 1. Public Surface:
    - Exports only PolicyAsCodeModule, BuildModule, and PolicyBridgeProvider.
    - Internal services, adapters, and ports stay behind internal/.
*/
package policyascode

import "context"

type ActiveWorkloadException struct {
	PublicID   string
	ApproverID string
}

type PolicyBridgeProvider interface {
	GetActiveExceptionForWorkload(ctx context.Context, tenantID string, namespace string, workloadName string) (*ActiveWorkloadException, error)
}
