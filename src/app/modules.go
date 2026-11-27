// Package app is the composition root that knows every feature module (Layer 3, L-03).
//
// Blueprint:
//  1. This is the ONLY package that imports more than one feature.
//  2. It wires capability bindings: kernel Authorizer → rbac module.
//  3. It wires bridges: consumer ports ← provider-exposed functions.
//  4. No business logic lives here — only construction and wiring.
//
// Invariants:
//   - Features are listed in dependency order (providers before consumers).
//   - Extracting any feature to its own service only changes this file and src/infra/clients/ (L-05).
package app

import (
	"internal-developer-platform/src/shared/kernel"
)

type Registry struct {
	modules []kernel.Module
}

func NewRegistry(modules ...kernel.Module) *Registry {
	return &Registry{modules: modules}
}

func (r *Registry) All() []kernel.Module {
	return r.modules
}

func (r *Registry) Routes() []kernel.Route {
	var out []kernel.Route
	for _, m := range r.modules {
		out = append(out, m.Routes()...)
	}
	return out
}

func (r *Registry) Lifecycles() []kernel.Lifecycle {
	var out []kernel.Lifecycle
	for _, m := range r.modules {
		out = append(out, m.Lifecycles()...)
	}
	return out
}
