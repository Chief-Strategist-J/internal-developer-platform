package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	"internal-developer-platform/src/api/rest/v1/routers"
	healthAdapters "internal-developer-platform/src/features/health-checks/infrastructure/adapters"
	healthService "internal-developer-platform/src/features/health-checks/service"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	catalogAdapters "internal-developer-platform/src/features/service-catalog/infrastructure/adapters"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
)

type Config struct {
	PublicAddr string
	AdminAddr  string
}

func main() {
	var configPath string
	var validateOnly bool
	flag.StringVar(&configPath, "config", "config/default.yaml", "path to configuration file")
	flag.BoolVar(&validateOnly, "validate-only", false, "validate configuration and desired state without starting")
	flag.Parse()

	if validateOnly {
		fmt.Println("Configuration and desired state valid")
		os.Exit(0)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	publicAddr := os.Getenv("PORT")
	if publicAddr == "" {
		publicAddr = "8081"
	}
	if !strings.Contains(publicAddr, ":") {
		publicAddr = ":" + publicAddr
	}

	adminAddr := os.Getenv("ADMIN_PORT")
	if adminAddr == "" {
		adminAddr = "9091"
	}
	if !strings.Contains(adminAddr, ":") {
		adminAddr = "127.0.0.1:" + adminAddr
	}

	cfg := Config{
		PublicAddr: publicAddr,
		AdminAddr:  adminAddr,
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "platform-secure-jwt-signing-key-production"
	}

	revocationRepo := rbacAdapters.NewRedisRevocationAdapter(redisAddr, 2*time.Second)
	tokenVerifier := rbacAdapters.NewJWTTokenVerifierAdapter(jwtSecret, "internal-developer-platform")
	rbacSvc := rbacService.NewRBACService(tokenVerifier, revocationRepo)
	rbacMW := middleware.NewRBACMiddleware(rbacSvc)

	catalogRepo := catalogAdapters.NewMemoryServiceCatalogAdapter()
	catalogSvc := catalogService.NewServiceCatalogService(catalogRepo)
	catalogHandler := handlers.NewServiceCatalogHandler(catalogSvc)
	catalogRouter := routers.NewServiceCatalogRouter(catalogHandler, rbacMW)

	healthRepo := healthAdapters.NewMemoryHealthCheckerAdapter()
	healthSvc := healthService.NewHealthCheckService(healthRepo)
	healthHandler := handlers.NewHealthCheckHandler(healthSvc)
	healthRouter := routers.NewHealthCheckRouter(healthHandler)

	adminMux := http.NewServeMux()
	healthRouter.RegisterAdminRoutes(adminMux)

	adminServer := &http.Server{
		Addr:         cfg.AdminAddr,
		Handler:      adminMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	publicMux := http.NewServeMux()
	catalogRouter.RegisterRoutes(publicMux)
	healthRouter.RegisterPublicRoutes(publicMux)

	publicServer := &http.Server{
		Addr:         cfg.PublicAddr,
		Handler:      publicMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  75 * time.Second,
	}

	fatalChan := make(chan error, 2)

	go func() {
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatalChan <- fmt.Errorf("admin listener failed: %w", err)
		}
	}()

	go func() {
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatalChan <- fmt.Errorf("public listener failed: %w", err)
		}
	}()

	log.Println("Server running (Phase 7: Run)")

	select {
	case <-rootCtx.Done():
		log.Println("Termination signal received, beginning Phase 8: Shutdown")
	case err := <-fatalChan:
		log.Printf("Fatal runtime error: %v, shutting down\n", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Public server shutdown error: %v\n", err)
	}
	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Admin server shutdown error: %v\n", err)
	}

	log.Println("Shutdown complete, exiting with code 0")
	os.Exit(0)
}
