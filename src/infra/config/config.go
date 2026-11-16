/*
Package config provides strongly typed, declarative configuration loading and schema validation
for the internal-developer-platform service. It loads baseline parameters from config/default.yaml,
applies validated environment variable overrides from .env / process environment, and verifies all
institutional invariants before service boot.

ZERO-INLINE-COMMENT DOCTRINE: Method bodies remain 100% comment-free and pure.
*/
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Name                    string `yaml:"name"`
	Port                    int    `yaml:"port"`
	AdminPort               int    `yaml:"admin_port"`
	IdleTimeoutSeconds      int    `yaml:"idle_timeout_seconds"`
	HeaderTimeoutSeconds    int    `yaml:"header_timeout_seconds"`
	DrainDeadlineSeconds    int    `yaml:"drain_deadline_seconds"`
	ShutdownDeadlineSeconds int    `yaml:"shutdown_deadline_seconds"`
}

type DatabaseConfig struct {
	URL                    string `yaml:"url"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes"`
}

type RedisConfig struct {
	Address        string `yaml:"address"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
	Issuer    string `yaml:"issuer"`
}

type KubernetesConfig struct {
	Endpoint string `yaml:"endpoint"`
}

type ArgoCDConfig struct {
	Endpoint string `yaml:"endpoint"`
	Token    string `yaml:"token"`
}

type EndpointsConfig struct {
	BasePath string `yaml:"base_path"`
}

type AppConfig struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Redis      RedisConfig      `yaml:"redis"`
	Auth       AuthConfig       `yaml:"auth"`
	Kubernetes KubernetesConfig `yaml:"kubernetes"`
	ArgoCD     ArgoCDConfig     `yaml:"argocd"`
	Endpoints  EndpointsConfig  `yaml:"endpoints"`
}

func LoadConfig(configPath string) (*AppConfig, error) {
	if configPath == "" {
		configPath = "config/default.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", configPath, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	applyEnvironmentOverrides(&cfg)

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func applyEnvironmentOverrides(cfg *AppConfig) {
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}

	if adminPortStr := os.Getenv("ADMIN_PORT"); adminPortStr != "" {
		if p, err := strconv.Atoi(adminPortStr); err == nil && p > 0 {
			cfg.Server.AdminPort = p
		}
	}

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		cfg.Database.URL = dbURL
	}

	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		cfg.Redis.Address = redisAddr
	}

	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		cfg.Auth.JWTSecret = jwtSecret
	}

	if issuer := os.Getenv("AUTH_ISSUER"); issuer != "" {
		cfg.Auth.Issuer = issuer
	}

	if k8sURL := os.Getenv("KUBERNETES_API_URL"); k8sURL != "" {
		cfg.Kubernetes.Endpoint = k8sURL
	}

	if argoURL := os.Getenv("ARGOCD_SERVER_URL"); argoURL != "" {
		cfg.ArgoCD.Endpoint = argoURL
	}

	if argoToken := os.Getenv("ARGOCD_TOKEN"); argoToken != "" {
		cfg.ArgoCD.Token = argoToken
	}
}

func validateConfig(cfg *AppConfig) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port %d out of valid range [1, 65535]", cfg.Server.Port)
	}

	if cfg.Server.AdminPort < 1 || cfg.Server.AdminPort > 65535 {
		return fmt.Errorf("admin port %d out of valid range [1, 65535]", cfg.Server.AdminPort)
	}

	if cfg.Server.Port == cfg.Server.AdminPort {
		return fmt.Errorf("server port and admin port must not collide: %d", cfg.Server.Port)
	}

	if strings.TrimSpace(cfg.Database.URL) == "" {
		return fmt.Errorf("database url must not be empty")
	}

	if strings.TrimSpace(cfg.Redis.Address) == "" {
		return fmt.Errorf("redis address must not be empty")
	}

	if len(cfg.Auth.JWTSecret) < 16 {
		return fmt.Errorf("jwt secret length must be at least 16 characters for security")
	}

	if strings.TrimSpace(cfg.Kubernetes.Endpoint) == "" {
		return fmt.Errorf("kubernetes endpoint must not be empty")
	}

	if strings.TrimSpace(cfg.ArgoCD.Endpoint) == "" {
		return fmt.Errorf("argocd endpoint must not be empty")
	}

	return nil
}

func (c *AppConfig) PublicBindAddress() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}

func (c *AppConfig) AdminBindAddress() string {
	return fmt.Sprintf("127.0.0.1:%d", c.Server.AdminPort)
}

