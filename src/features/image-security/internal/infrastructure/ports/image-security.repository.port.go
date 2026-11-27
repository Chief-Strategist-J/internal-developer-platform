package ports

import (
	"context"

	"internal-developer-platform/src/features/image-security/internal/contracts"
)

type ImageSecurityScannerPort interface {
	ScanImage(ctx context.Context, repository string, digest string) ([]contracts.VulnerabilityItem, []contracts.EmbeddedSecretFinding, error)
	GetSBOMAttestation(ctx context.Context, digest string) (*contracts.SBOMAttestation, error)
}

type SecurityWaiverRepositoryPort interface {
	GetActiveWaiver(ctx context.Context, imageDigest string, cveID string) (*contracts.SecurityExceptionWaiver, error)
	SaveWaiver(ctx context.Context, waiver contracts.SecurityExceptionWaiver) error
}
