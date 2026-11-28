/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES INDEX (V.3 role 13, X-05)

1. Role:
   - Public feature entrypoint exporting Module constructor and types only.
2. Invariants:
   - Zero internal domain services, ports, or adapters exported outside the feature.
   - Zero inline comments below header.
*/

package deploymentstrategies

func NewModule(deps ModuleDeps) *DeploymentStrategiesModule {
	return BuildModule(deps)
}
