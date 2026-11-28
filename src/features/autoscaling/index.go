/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING INDEX (V.3 role 13, X-05)

1. Role:
   - Public feature entrypoint exporting the Module constructor only.
2. Invariants:
   - Zero internal services, rules or contracts exported outside the feature.
   - Zero inline comments below this header.
*/

package autoscaling

func NewModule(deps ModuleDeps) *AutoscalingModule {
	return BuildModule(deps)
}
