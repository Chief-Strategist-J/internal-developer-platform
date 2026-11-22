package imagesecurity

import (
	"internal-developer-platform/src/features/image-security/contracts"
	"internal-developer-platform/src/features/image-security/infrastructure/ports"
	"internal-developer-platform/src/features/image-security/service"
)

type Service = service.ImageSecurityService
type ScannerPort = ports.ImageSecurityScannerPort
type WaiverPort = ports.SecurityWaiverRepositoryPort
type Verdict = contracts.ImageAdmissionVerdict
type Command = contracts.ImageScanCommand

func NewService(scanner ScannerPort, waivers WaiverPort) *Service {
	return service.NewImageSecurityService(scanner, waivers)
}
