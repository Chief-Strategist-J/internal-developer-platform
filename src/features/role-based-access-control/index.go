// Package rbac is the public surface of the role-based-access-control feature (V.3 role 13, X-05).
//
// Blueprint:
//  1. Only Module, provided capability types, and bridge-facing exposes are exported.
//  2. All internal types (Service, adapters, ports) are hidden behind the module boundary.
//  3. src/app/ accesses the RBAC service only via RBACModule.Service() for bridge wiring.
//
// Invariants:
//   - No feature may import this package directly (import-boss enforced, G-02).
//   - Only src/app/ and the kernel bridge may import this package's exported symbols.
package rbac

// Re-export the module constructor as the package's primary entry point.
// All other types in this package (service, ports, adapters) are unexported or internal.
//
// Usage in src/app/modules.go:
//
//	rbacMod := rbac.NewRBACModule(svc, routes, lifecycles)
//	registry := app.NewRegistry(rbacMod, gitopsMod, ...)
