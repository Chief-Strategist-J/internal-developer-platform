package chassis

import (
	"context"
	"database/sql"
	"net/http"

	"internal-developer-platform/src/infra/config"
	"internal-developer-platform/src/infra/observability"
)

type Criticality string

const (
	CriticalityHard Criticality = "hard"
	CriticalitySoft Criticality = "soft"
)

type Identity struct {
	Name    string
	Team    string
	Tier    string
	Version string
}

type InfrastructureNeeds struct {
	Database bool
	Redis    bool
	Producer bool
	Consumer bool
	Flags    bool
}

type DownstreamDependency struct {
	Name        string
	Criticality Criticality
}

type ServiceContracts struct {
	ApiSchemas     []string
	ProducedTopics []string
	ConsumedTopics []string
}

type Dependencies struct {
	Config *config.AppConfig
	DB     *sql.DB
	Logger *observability.Logger
	Tracer *observability.Tracer
}

type ServiceSpec struct {
	Identity            func() Identity
	Needs               func() InfrastructureNeeds
	DependsOn           func() []DownstreamDependency
	Contracts           func() ServiceContracts
	BuildServices       func(deps Dependencies) (interface{}, error)
	RegisterApiRoutes        func(mux *http.ServeMux, services interface{})
	RegisterAdminRoutes      func(mux *http.ServeMux, services interface{})
	StartBackgroundWorkers   func(ctx context.Context, services interface{})
	ExecuteCommand           func(ctx context.Context, command string, args []string, services interface{}) (int, bool)
}
