/*
Package config tests verify loading, environment variable overrides, and declarative validation
for the internal-developer-platform service configuration.
*/
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefault(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	configPath := filepath.Join(wd, "../../../config/default.yaml")
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading default config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default server port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.AdminPort != 8081 {
		t.Errorf("expected default admin port 8081, got %d", cfg.Server.AdminPort)
	}
	if cfg.Kubernetes.Endpoint != "https://k8s-api.internal:6443" {
		t.Errorf("expected default k8s endpoint, got %s", cfg.Kubernetes.Endpoint)
	}
	if cfg.ArgoCD.Endpoint != "https://argocd.internal:443" {
		t.Errorf("expected default argocd endpoint, got %s", cfg.ArgoCD.Endpoint)
	}
}

func TestLoadConfigEnvironmentOverrides(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	t.Setenv("PORT", "9999")
	t.Setenv("ADMIN_PORT", "9998")
	t.Setenv("KUBERNETES_API_URL", "https://k8s-prod.gke:443")
	t.Setenv("ARGOCD_SERVER_URL", "https://argo-prod.internal:443")
	t.Setenv("ARGOCD_TOKEN", "secret-token-12345")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "custom-gcp-project")
	t.Setenv("CLOUD_SQL_ADMIN_BASE_URL", "https://custom-sqladmin.internal")
	t.Setenv("CLOUD_DNS_BASE_URL", "https://custom-dns.internal")

	configPath := filepath.Join(wd, "../../../config/default.yaml")
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading config with overrides: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("expected overridden port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.AdminPort != 9998 {
		t.Errorf("expected overridden admin port 9998, got %d", cfg.Server.AdminPort)
	}
	if cfg.Kubernetes.Endpoint != "https://k8s-prod.gke:443" {
		t.Errorf("expected overridden k8s endpoint, got %s", cfg.Kubernetes.Endpoint)
	}
	if cfg.ArgoCD.Endpoint != "https://argo-prod.internal:443" {
		t.Errorf("expected overridden argocd endpoint, got %s", cfg.ArgoCD.Endpoint)
	}
	if cfg.ArgoCD.Token != "secret-token-12345" {
		t.Errorf("expected overridden argocd token, got %s", cfg.ArgoCD.Token)
	}

	if cfg.GCP.ProjectID != "custom-gcp-project" {
		t.Errorf("expected overridden gcp project_id, got %s", cfg.GCP.ProjectID)
	}
	if cfg.GCP.CloudSQLAdminURL != "https://custom-sqladmin.internal" {
		t.Errorf("expected overridden cloud sql url, got %s", cfg.GCP.CloudSQLAdminURL)
	}
	if cfg.GCP.CloudDNSURL != "https://custom-dns.internal" {
		t.Errorf("expected overridden cloud dns url, got %s", cfg.GCP.CloudDNSURL)
	}
}

func TestValidateConfigPortCollision(t *testing.T) {
	cfg := &AppConfig{
		Server: ServerConfig{
			Port:      8080,
			AdminPort: 8080,
		},
		Database: DatabaseConfig{
			URL: "postgres://localhost:5432/db",
		},
		Redis: RedisConfig{
			Address: "localhost:6379",
		},
		Auth: AuthConfig{
			JWTSecret: "super-secure-secret-key-12345",
			Issuer:    "test-issuer",
		},
		Kubernetes: KubernetesConfig{
			Endpoint: "https://k8s.internal:6443",
		},
		ArgoCD: ArgoCDConfig{
			Endpoint: "https://argo.internal:443",
		},
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Errorf("expected error on port collision")
	}
}

func TestValidateConfigShortSecret(t *testing.T) {
	cfg := &AppConfig{
		Server: ServerConfig{
			Port:      8080,
			AdminPort: 8081,
		},
		Database: DatabaseConfig{
			URL: "postgres://localhost:5432/db",
		},
		Redis: RedisConfig{
			Address: "localhost:6379",
		},
		Auth: AuthConfig{
			JWTSecret: "short",
			Issuer:    "test-issuer",
		},
		Kubernetes: KubernetesConfig{
			Endpoint: "https://k8s.internal:6443",
		},
		ArgoCD: ArgoCDConfig{
			Endpoint: "https://argo.internal:443",
		},
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Errorf("expected error on short JWT secret")
	}
}
