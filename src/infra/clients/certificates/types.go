package certificates

import (
	"time"
)

type IssuerKind string

const (
	IssuerKindNamespaced IssuerKind = "Issuer"
	IssuerKindCluster    IssuerKind = "ClusterIssuer"
)

type IssuerType string

const (
	IssuerTypeACME IssuerType = "ACME"
	IssuerTypeCA   IssuerType = "CA"
)

type KeyAlgorithm string

const (
	KeyAlgorithmECDSA KeyAlgorithm = "ECDSA"
	KeyAlgorithmRSA   KeyAlgorithm = "RSA"
)

const (
	CertManagerAPIVersion = "cert-manager.io/v1"
	CertManagerGroup      = "cert-manager.io"
	DefaultIngressClass   = "istio"
	DefaultCertDuration   = 2160 * time.Hour
	DefaultRenewBefore    = 720 * time.Hour
	DefaultECDSASize      = 256
	DefaultRSASize        = 4096
)

type CertificateSpec struct {
	Name         string
	Namespace    string
	SecretName   string
	DNSNames     []string
	IssuerName   string
	IssuerKind   IssuerKind
	KeyAlgorithm KeyAlgorithm
	KeySize      int
	Duration     time.Duration
	RenewBefore  time.Duration
}

type IssuerSpec struct {
	Name               string
	Namespace          string
	Type               IssuerType
	ServerURL          string
	Email              string
	CAServerSecretName string
}
