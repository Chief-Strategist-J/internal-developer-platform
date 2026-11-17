package certificates

import (
	"fmt"
	"strings"

	"internal-developer-platform/src/shared/errors"
)

func GenerateCertificateYAML(spec CertificateSpec) (string, error) {
	if spec.Name == "" || spec.Namespace == "" || spec.SecretName == "" {
		return "", errors.NewValidationError("name, namespace, and secretName are required")
	}
	if len(spec.DNSNames) == 0 {
		return "", errors.NewValidationError("at least one DNS name is required")
	}
	if spec.IssuerName == "" {
		return "", errors.NewValidationError("issuerName is required")
	}

	issuerKind := spec.IssuerKind
	if issuerKind == "" {
		issuerKind = IssuerKindCluster
	}

	keyAlgo := spec.KeyAlgorithm
	if keyAlgo == "" {
		keyAlgo = KeyAlgorithmECDSA
	}

	keySize := spec.KeySize
	if keySize == 0 {
		if keyAlgo == KeyAlgorithmECDSA {
			keySize = DefaultECDSASize
		} else {
			keySize = DefaultRSASize
		}
	}

	duration := spec.Duration
	if duration <= 0 {
		duration = DefaultCertDuration
	}

	renewBefore := spec.RenewBefore
	if renewBefore <= 0 {
		renewBefore = DefaultRenewBefore
	}

	var dnsList strings.Builder
	for _, dns := range spec.DNSNames {
		dnsList.WriteString(fmt.Sprintf("    - %s\n", dns))
	}

	manifest := fmt.Sprintf(`apiVersion: %s
kind: Certificate
metadata:
  name: %s
  namespace: %s
spec:
  secretName: %s
  duration: %dh
  renewBefore: %dh
  dnsNames:
%s  issuerRef:
    name: %s
    kind: %s
    group: %s
  privateKey:
    algorithm: %s
    size: %d
`,
		CertManagerAPIVersion,
		spec.Name,
		spec.Namespace,
		spec.SecretName,
		int(duration.Hours()),
		int(renewBefore.Hours()),
		dnsList.String(),
		spec.IssuerName,
		issuerKind,
		CertManagerGroup,
		keyAlgo,
		keySize,
	)

	return manifest, nil
}

func GenerateIssuerYAML(spec IssuerSpec) (string, error) {
	if spec.Name == "" || spec.Namespace == "" {
		return "", errors.NewValidationError("name and namespace are required")
	}

	if spec.Type == IssuerTypeACME {
		if spec.ServerURL == "" || spec.Email == "" {
			return "", errors.NewValidationError("serverURL and email are required for ACME issuer")
		}
		manifest := fmt.Sprintf(`apiVersion: %s
kind: Issuer
metadata:
  name: %s
  namespace: %s
spec:
  acme:
    server: %s
    email: %s
    privateKeySecretRef:
      name: %s-acme-account-key
    solvers:
      - http01:
          ingress:
            class: %s
`,
			CertManagerAPIVersion,
			spec.Name,
			spec.Namespace,
			spec.ServerURL,
			spec.Email,
			spec.Name,
			DefaultIngressClass,
		)
		return manifest, nil
	}

	if spec.Type == IssuerTypeCA {
		if spec.CAServerSecretName == "" {
			return "", errors.NewValidationError("caServerSecretName is required for CA issuer")
		}
		manifest := fmt.Sprintf(`apiVersion: %s
kind: Issuer
metadata:
  name: %s
  namespace: %s
spec:
  ca:
    secretName: %s
`,
			CertManagerAPIVersion,
			spec.Name,
			spec.Namespace,
			spec.CAServerSecretName,
		)
		return manifest, nil
	}

	return "", errors.NewValidationError("unsupported issuer type: " + string(spec.Type))
}
