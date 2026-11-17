package certificates

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateCertificateYAMLDefaultValues(t *testing.T) {
	spec := CertificateSpec{
		Name:       "frontend-cert",
		Namespace:  "tenant-prod",
		SecretName: "frontend-tls-secret",
		DNSNames:   []string{"app.example.com", "api.example.com"},
		IssuerName: "letsencrypt-prod",
	}

	yaml, err := GenerateCertificateYAML(spec)
	if err != nil {
		t.Fatalf("unexpected error generating cert yaml: %v", err)
	}

	if !strings.Contains(yaml, "cert-manager.io/v1") {
		t.Errorf("expected apiVersion cert-manager.io/v1")
	}
	if !strings.Contains(yaml, "duration: 2160h") {
		t.Errorf("expected default duration 2160h (90 days)")
	}
	if !strings.Contains(yaml, "renewBefore: 720h") {
		t.Errorf("expected default renewBefore 720h (30 days)")
	}
	if !strings.Contains(yaml, "algorithm: ECDSA") {
		t.Errorf("expected default algorithm ECDSA")
	}
	if !strings.Contains(yaml, "size: 256") {
		t.Errorf("expected default size 256")
	}
	if !strings.Contains(yaml, "app.example.com") {
		t.Errorf("expected DNS name app.example.com")
	}
}

func TestGenerateCertificateYAMLCustomRSA(t *testing.T) {
	spec := CertificateSpec{
		Name:         "payment-cert",
		Namespace:    "tenant-prod",
		SecretName:   "payment-tls-secret",
		DNSNames:     []string{"payment.internal"},
		IssuerName:   "mesh-ca-issuer",
		IssuerKind:   IssuerKindNamespaced,
		KeyAlgorithm: KeyAlgorithmRSA,
		KeySize:      4096,
		Duration:     1080 * time.Hour,
		RenewBefore:  360 * time.Hour,
	}

	yaml, err := GenerateCertificateYAML(spec)
	if err != nil {
		t.Fatalf("unexpected error generating cert yaml: %v", err)
	}

	if !strings.Contains(yaml, "kind: Issuer") {
		t.Errorf("expected namespaced Issuer kind")
	}
	if !strings.Contains(yaml, "algorithm: RSA") {
		t.Errorf("expected algorithm RSA")
	}
	if !strings.Contains(yaml, "size: 4096") {
		t.Errorf("expected size 4096")
	}
	if !strings.Contains(yaml, "duration: 1080h") {
		t.Errorf("expected custom duration 1080h")
	}
}

func TestGenerateIssuerYAML(t *testing.T) {
	acmeSpec := IssuerSpec{
		Name:      "letsencrypt-acme",
		Namespace: "tenant-prod",
		Type:      IssuerTypeACME,
		ServerURL: "https://acme-v02.api.letsencrypt.org/directory",
		Email:     "sec-ops@company.internal",
	}

	acmeYAML, err := GenerateIssuerYAML(acmeSpec)
	if err != nil {
		t.Fatalf("failed to generate acme issuer: %v", err)
	}
	if !strings.Contains(acmeYAML, "https://acme-v02.api.letsencrypt.org/directory") {
		t.Errorf("expected acme server url")
	}

	caSpec := IssuerSpec{
		Name:               "internal-ca",
		Namespace:          "tenant-prod",
		Type:               IssuerTypeCA,
		CAServerSecretName: "root-ca-key-pair",
	}

	caYAML, err := GenerateIssuerYAML(caSpec)
	if err != nil {
		t.Fatalf("failed to generate ca issuer: %v", err)
	}
	if !strings.Contains(caYAML, "secretName: root-ca-key-pair") {
		t.Errorf("expected ca secret name")
	}
}

func TestGenerateCertificateValidationErrors(t *testing.T) {
	_, err := GenerateCertificateYAML(CertificateSpec{})
	if err == nil {
		t.Fatalf("expected error for empty certificate spec")
	}

	_, err = GenerateIssuerYAML(IssuerSpec{})
	if err == nil {
		t.Fatalf("expected error for empty issuer spec")
	}
}

