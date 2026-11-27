/*
ALGORITHM & ARCHITECTURE BLUEPRINT: IMAGE SECURITY MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles ImageSecurityService with its scanner/waiver ports and satisfies kernel.Module.
 2. Invariants:
    - Zero goroutines or tickers in constructors (G-11).
    - Zero comments below this header.
*/
package imagesecurity

import (
	"internal-developer-platform/src/features/image-security/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/image-security/internal/infrastructure/ports"
	"internal-developer-platform/src/features/image-security/internal/service"
	"internal-developer-platform/src/shared/kernel"
)

type ImageSecurityModule struct {
	svc *service.ImageSecurityService
}

func BuildModule(waivers ports.SecurityWaiverRepositoryPort) *ImageSecurityModule {
	scanner := adapters.NewMemoryImageScannerAdapter()
	svc := service.NewImageSecurityService(scanner, waivers)
	return &ImageSecurityModule{svc: svc}
}

func (m *ImageSecurityModule) Name() string                         { return "image-security" }
func (m *ImageSecurityModule) Routes() []kernel.Route               { return nil }
func (m *ImageSecurityModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *ImageSecurityModule) Lifecycles() []kernel.Lifecycle       { return nil }
func (m *ImageSecurityModule) Provides() []string                   { return nil }
