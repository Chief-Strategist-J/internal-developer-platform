package service

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/image-security/internal/contracts"
	"internal-developer-platform/src/features/image-security/internal/domain/workflows"
	"internal-developer-platform/src/features/image-security/internal/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
)

type ImageSecurityService struct {
	scanner ports.ImageSecurityScannerPort
	waivers ports.SecurityWaiverRepositoryPort
}

func NewImageSecurityService(scanner ports.ImageSecurityScannerPort, waivers ports.SecurityWaiverRepositoryPort) *ImageSecurityService {
	return &ImageSecurityService{
		scanner: scanner,
		waivers: waivers,
	}
}

func (s *ImageSecurityService) AssessImageAdmission(ctx context.Context, cmd contracts.ImageScanCommand) (contracts.ImageAdmissionVerdict, error) {
	return observability.RunTraced(ctx, "image_security.assess_admission", func(ctx context.Context) (contracts.ImageAdmissionVerdict, error) {
		if err := contracts.ValidateImageScanCommand(cmd); err != nil {
			return contracts.ImageAdmissionVerdict{}, err
		}

		if s.scanner != nil && len(cmd.Vulnerabilities) == 0 {
			cves, secrets, err := s.scanner.ScanImage(ctx, cmd.ImageRepository, cmd.ImageDigest)
			if err == nil {
				cmd.Vulnerabilities = cves
				cmd.SecretsFound = secrets
			}
			if cmd.SBOM == nil {
				sbom, sbomErr := s.scanner.GetSBOMAttestation(ctx, cmd.ImageDigest)
				if sbomErr == nil {
					cmd.SBOM = sbom
				}
			}
		}

		verdict, err := workflows.ExecuteImageSecurityWorkflow(ctx, cmd, time.Now().UTC())
		if err != nil {
			return contracts.ImageAdmissionVerdict{}, err
		}
		return verdict, nil
	}, attribute.String("image.digest", cmd.ImageDigest), attribute.String("namespace", cmd.Namespace))
}
