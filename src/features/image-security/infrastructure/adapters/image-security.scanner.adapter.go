package adapters

import (
	"context"
	"sync"
	"time"

	"internal-developer-platform/src/features/image-security/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryImageScannerAdapter struct {
	mu           sync.RWMutex
	cves         map[string][]contracts.VulnerabilityItem
	secrets      map[string][]contracts.EmbeddedSecretFinding
	sboms        map[string]contracts.SBOMAttestation
	offlineCache map[string]time.Time
}

func NewMemoryImageScannerAdapter() *MemoryImageScannerAdapter {
	return &MemoryImageScannerAdapter{
		cves:         make(map[string][]contracts.VulnerabilityItem),
		secrets:      make(map[string][]contracts.EmbeddedSecretFinding),
		sboms:        make(map[string]contracts.SBOMAttestation),
		offlineCache: make(map[string]time.Time),
	}
}

func (a *MemoryImageScannerAdapter) SetImageCVEs(digest string, items []contracts.VulnerabilityItem) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cves[digest] = items
	a.offlineCache[digest] = time.Now().UTC()
}

func (a *MemoryImageScannerAdapter) SetImageSecrets(digest string, secrets []contracts.EmbeddedSecretFinding) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.secrets[digest] = secrets
}

func (a *MemoryImageScannerAdapter) SetImageSBOM(digest string, sbom contracts.SBOMAttestation) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sboms[digest] = sbom
}

func (a *MemoryImageScannerAdapter) ScanImage(ctx context.Context, repository string, digest string) ([]contracts.VulnerabilityItem, []contracts.EmbeddedSecretFinding, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cveList := a.cves[digest]
	secretList := a.secrets[digest]
	return cveList, secretList, nil
}

func (a *MemoryImageScannerAdapter) GetSBOMAttestation(ctx context.Context, digest string) (*contracts.SBOMAttestation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	sbom, exists := a.sboms[digest]
	if !exists {
		return nil, errors.NewNotFoundError("SBOM attestation not found for digest")
	}
	return &sbom, nil
}
