package chassis

import (
	"context"
	"net/http"
	"testing"

	"internal-developer-platform/src/infra/observability"
)

func TestChassisPreflightValidFlags(t *testing.T) {
	args := []string{"-config", "config/default.yaml", "-validate-only"}
	bootCtx, code := preflight(args)
	if code != ExitSuccess {
		t.Fatalf("expected ExitSuccess (0), got %d", code)
	}
	defer bootCtx.cancel()

	if bootCtx.configPath != "config/default.yaml" {
		t.Errorf("expected config path 'config/default.yaml', got '%s'", bootCtx.configPath)
	}
	if !bootCtx.validateOnly {
		t.Error("expected validateOnly to be true")
	}
}

func TestChassisPreflightInvalidFlag(t *testing.T) {
	args := []string{"-unknown-flag"}
	_, code := preflight(args)
	if code != ExitUsage {
		t.Fatalf("expected ExitUsage (2), got %d", code)
	}
}

func TestChassisRunValidateOnly(t *testing.T) {
	spec := ServiceSpec{
		Identity: func() Identity {
			return Identity{
				Name:    "test-service",
				Team:    "platform-core",
				Tier:    "tier-1",
				Version: "1.0.0",
			}
		},
		Needs: func() InfrastructureNeeds {
			return InfrastructureNeeds{Database: false}
		},
		DependsOn: func() []DownstreamDependency {
			return nil
		},
		Contracts: func() ServiceContracts {
			return ServiceContracts{}
		},
		BuildServices: func(deps Dependencies) (interface{}, error) {
			return "mock-services", nil
		},
		RegisterApiRoutes: func(mux *http.ServeMux, services interface{}) {
			mux.HandleFunc("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		},
	}

	code := Run(spec, []string{"-config", "../../../config/default.yaml", "-validate-only"}, nil)
	if code != ExitSuccess {
		t.Fatalf("expected ExitSuccess, got exit code %d", code)
	}
}

func TestChassisConfigureInvalidPath(t *testing.T) {
	logger := observability.NewLogger(&testWriter{}, "test")
	bootCtx := &bootContext{
		configPath: "non-existent-config.yaml",
	}
	spec := ServiceSpec{}

	_, code := configure(bootCtx, spec, logger, context.Background())
	if code != ExitConfig {
		t.Fatalf("expected ExitConfig (3), got %d", code)
	}
}

type testWriter struct{}

func (w *testWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
