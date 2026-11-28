package chassis

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"internal-developer-platform/src/infra/config"
	"internal-developer-platform/src/infra/observability"
)

const (
	ExitSuccess    = 0
	ExitUsage      = 2
	ExitConfig     = 3
	ExitDependency = 4
	ExitRuntime    = 5
	ExitTimeout    = 6
)

type bootContext struct {
	configPath   string
	validateOnly bool
	command      string
	cmdArgs      []string
	rootCtx      context.Context
	cancel       context.CancelFunc
}

type composedApp struct {
	config   *config.AppConfig
	db       *sql.DB
	services interface{}
	tpCancel func(context.Context) error
}

func Run(spec ServiceSpec, args []string, env []string) int {
	logger := observability.GetLogger()
	ctx := context.Background()

	bootCtx, exitCode := preflight(args)
	if exitCode != ExitSuccess {
		return exitCode
	}
	defer bootCtx.cancel()

	cfg, exitCode := configure(bootCtx, spec, logger, ctx)
	if exitCode != ExitSuccess {
		return exitCode
	}
	if bootCtx.validateOnly {
		logger.Info(ctx, "configuration validation passed successfully", observability.String("status", "valid"))
		return ExitSuccess
	}

	app, exitCode := compose(bootCtx.rootCtx, cfg, spec, logger)
	if exitCode != ExitSuccess {
		return exitCode
	}
	if app.tpCancel != nil {
		defer func() {
			_ = app.tpCancel(context.Background())
		}()
	}

	if bootCtx.command != "" && spec.ExecuteCommand != nil {
		if code, handled := spec.ExecuteCommand(bootCtx.rootCtx, bootCtx.command, bootCtx.cmdArgs, app.services); handled {
			return code
		}
	}

	if spec.StartBackgroundWorkers != nil {
		spec.StartBackgroundWorkers(bootCtx.rootCtx, app.services)
	}

	publicServer, adminServer, metricsServer := wireCommand(app, cfg, spec)

	stopReason := runLongLived(bootCtx.rootCtx, publicServer, adminServer, metricsServer, cfg, logger)

	return shutdown(publicServer, adminServer, metricsServer, cfg, logger, stopReason)
}

func preflight(args []string) (*bootContext, int) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		return &bootContext{
			configPath:   "config/default.yaml",
			validateOnly: false,
			command:      args[0],
			cmdArgs:      args[1:],
			rootCtx:      rootCtx,
			cancel:       cancel,
		}, ExitSuccess
	}

	fs := flag.NewFlagSet("chassis", flag.ContinueOnError)
	var configPath string
	var validateOnly bool
	fs.StringVar(&configPath, "config", "config/default.yaml", "path to configuration file")
	fs.BoolVar(&validateOnly, "validate-only", false, "validate configuration and exit")

	if err := fs.Parse(args); err != nil {
		return nil, ExitUsage
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	return &bootContext{
		configPath:   configPath,
		validateOnly: validateOnly,
		command:      "",
		cmdArgs:      nil,
		rootCtx:      rootCtx,
		cancel:       cancel,
	}, ExitSuccess
}

func configure(bootCtx *bootContext, spec ServiceSpec, logger *observability.Logger, ctx context.Context) (*config.AppConfig, int) {
	cfg, err := config.LoadConfig(bootCtx.configPath)
	if err != nil {
		logger.Error(ctx, "configuration loading failed", observability.String("error", err.Error()), observability.String("path", bootCtx.configPath))
		return nil, ExitConfig
	}
	return cfg, ExitSuccess
}

func compose(ctx context.Context, cfg *config.AppConfig, spec ServiceSpec, logger *observability.Logger) (*composedApp, int) {
	identity := spec.Identity()
	_, tpShutdown, err := observability.InitTracer(identity.Name, cfg.Observability.OTLPEndpoint)
	if err != nil {
		logger.Error(ctx, "failed to initialize tracer provider", observability.String("error", err.Error()))
		return nil, ExitDependency
	}

	var dbConn *sql.DB
	needs := spec.Needs()
	if needs.Database && cfg.Database.URL != "" && !strings.Contains(cfg.Database.URL, "memory") {
		var openErr error
		dbConn, openErr = sql.Open("postgres", cfg.Database.URL)
		if openErr != nil {
			logger.Error(ctx, "failed to open database connection pool", observability.String("error", openErr.Error()))
			return nil, ExitDependency
		}
		dbConn.SetMaxOpenConns(cfg.Database.MaxOpenConns)
		dbConn.SetMaxIdleConns(cfg.Database.MaxIdleConns)
		dbConn.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute)

		if pingErr := dbConn.Ping(); pingErr != nil {
			logger.Warn(ctx, "database ping failed; soft dependency fallback active", observability.String("error", pingErr.Error()))
			_ = dbConn.Close()
			dbConn = nil
		} else {
			logger.Info(ctx, "connected to live postgres database pool", observability.Int("max_open_conns", cfg.Database.MaxOpenConns))
		}
	}

	deps := Dependencies{
		Config: cfg,
		DB:     dbConn,
		Logger: logger,
		Tracer: observability.GetTracer(),
	}

	services, buildErr := spec.BuildServices(deps)
	if buildErr != nil {
		logger.Error(ctx, "domain services build failed", observability.String("error", buildErr.Error()))
		return nil, ExitDependency
	}

	return &composedApp{
		config:   cfg,
		db:       dbConn,
		services: services,
		tpCancel: tpShutdown,
	}, ExitSuccess
}

func wireCommand(app *composedApp, cfg *config.AppConfig, spec ServiceSpec) (*http.Server, *http.Server, *http.Server) {
	metricsHandler := observability.MetricsHandler()

	adminMux := http.NewServeMux()
	if spec.RegisterAdminRoutes != nil {
		spec.RegisterAdminRoutes(adminMux, app.services)
	}
	adminMux.Handle("/metrics", metricsHandler)

	adminServer := &http.Server{
		Addr:         cfg.AdminBindAddress(),
		Handler:      adminMux,
		ReadTimeout:  time.Duration(cfg.Server.HeaderTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.HeaderTimeoutSeconds) * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metricsHandler)

	metricsServer := &http.Server{
		Addr:         cfg.MetricsBindAddress(),
		Handler:      metricsMux,
		ReadTimeout:  time.Duration(cfg.Server.HeaderTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.HeaderTimeoutSeconds) * time.Second,
	}

	publicMux := http.NewServeMux()
	if spec.RegisterApiRoutes != nil {
		spec.RegisterApiRoutes(publicMux, app.services)
	}

	telemetryWrappedMux := observability.IngressTelemetryMiddleware(publicMux)

	publicServer := &http.Server{
		Addr:         cfg.PublicBindAddress(),
		Handler:      telemetryWrappedMux,
		ReadTimeout:  time.Duration(cfg.Server.HeaderTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.HeaderTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
	}

	return publicServer, adminServer, metricsServer
}

func runLongLived(rootCtx context.Context, publicServer *http.Server, adminServer *http.Server, metricsServer *http.Server, cfg *config.AppConfig, logger *observability.Logger) string {
	fatalChan := make(chan error, 3)

	go func() {
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatalChan <- fmt.Errorf("admin listener failed: %w", err)
		}
	}()

	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatalChan <- fmt.Errorf("metrics listener failed: %w", err)
		}
	}()

	go func() {
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatalChan <- fmt.Errorf("public listener failed: %w", err)
		}
	}()

	logger.Info(rootCtx, "chassis servers running",
		observability.String("public_addr", cfg.PublicBindAddress()),
		observability.String("admin_addr", cfg.AdminBindAddress()),
		observability.String("metrics_addr", cfg.MetricsBindAddress()),
	)

	select {
	case <-rootCtx.Done():
		return "SIGTERM/SIGINT"
	case err := <-fatalChan:
		logger.Error(rootCtx, "listener fatal error", observability.String("error", err.Error()))
		return "FATAL_LISTENER"
	}
}

func shutdown(publicServer *http.Server, adminServer *http.Server, metricsServer *http.Server, cfg *config.AppConfig, logger *observability.Logger, stopReason string) int {
	ctx := context.Background()
	logger.Info(ctx, "beginning graceful shutdown", observability.String("reason", stopReason))

	drainTimeout := time.Duration(cfg.Server.ShutdownDeadlineSeconds) * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()

	hasError := false
	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "public server graceful drain failed", observability.String("error", err.Error()))
		hasError = true
	}

	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "admin server graceful drain failed", observability.String("error", err.Error()))
		hasError = true
	}

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "metrics server graceful drain failed", observability.String("error", err.Error()))
		hasError = true
	}

	if hasError {
		return ExitTimeout
	}

	logger.Info(ctx, "graceful shutdown completed successfully", observability.Int("exit_code", ExitSuccess))
	return ExitSuccess
}
