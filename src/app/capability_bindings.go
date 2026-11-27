// Package app: capability-bindings declares which feature provides each kernel capability (X-02).
//
// Blueprint:
//  1. Exactly one provider per capability per process role (X-02 enforced by check).
//  2. This file is updated whenever a feature starts or stops providing a capability.
//  3. Bridges in bridges/ implement consumer ports using the provider's exposed functions.
//
// Invariants:
//   - No business logic. This file is a wiring table only.
package app

import (
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

// CapabilityBindings maps each kernel capability to its single provider.
// At boot, main.go reads these and passes the provider implementation
// to every module that declares asks: [<capability>] in its context.yml.
type CapabilityBindings struct {
	Authorizer authz.Authorizer
}
