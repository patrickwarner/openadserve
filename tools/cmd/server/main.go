package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/patrickwarner/openadserve/internal/analytics"
	"github.com/patrickwarner/openadserve/internal/api"
	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/geoip"
	"github.com/patrickwarner/openadserve/internal/logic/ratelimit"
	"github.com/patrickwarner/openadserve/internal/logic/selectors"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/optimization"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	logger, err := observability.InitLoggerWithService(cfg.ServiceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
		}
	}()

	if err := run(logger, cfg); err != nil {
		logger.Error("server error", zap.Error(err))
		os.Exit(1)
	}
}

func run(logger *zap.Logger, cfg config.Config) error {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := db.InitPostgres(cfg.PostgresDSN, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	if err != nil {
		return fmt.Errorf("failed to connect postgres: %w", err)
	}
	defer pg.Close()

	// Initialize AdDataStore first, before loading any data
	adDataStore := models.NewInMemoryAdDataStore()
	if adDataStore == nil {
		return fmt.Errorf("failed to initialize ad data store")
	}

	// Load data from Postgres
	items, err := pg.LoadLineItems()
	if err != nil {
		return fmt.Errorf("load line items: %w", err)
	}

	campaigns, err := pg.LoadCampaigns()
	if err != nil {
		return fmt.Errorf("load campaigns: %w", err)
	}

	publishers, err := pg.LoadPublishers()
	if err != nil {
		return fmt.Errorf("load publishers: %w", err)
	}

	placements, err := pg.LoadPlacements()
	if err != nil {
		return fmt.Errorf("load placements: %w", err)
	}

	// Use AdDataStore's atomic ReloadAll to populate all data at once
	if err := adDataStore.ReloadAll(items, campaigns, publishers, placements); err != nil {
		return fmt.Errorf("populate ad data store: %w", err)
	}

	database, err := db.Init(pg, adDataStore)
	if err != nil {
		return fmt.Errorf("failed to load db: %w", err)
	}
	store, err := db.InitRedis(cfg.RedisAddr)
	if err != nil {
		return fmt.Errorf("failed to connect redis: %w", err)
	}
	defer store.Close()

	// Initialize metrics registry
	metricsRegistry := observability.NewPrometheusRegistry()

	analyticsSvc, err := analytics.InitClickHouse(cfg.ClickHouseDSN, pg, metricsRegistry)
	if err != nil {
		return fmt.Errorf("failed to connect clickhouse: %w", err)
	}
	defer analyticsSvc.Close()

	geoSvc, err := geoip.Init(cfg.GeoIPDB)
	if err != nil {
		return fmt.Errorf("failed to load geoip db: %w", err)
	}
	defer func() { _ = geoSvc.Close() }()

	// Initialize rate limiter
	rateLimiterConfig := ratelimit.Config{
		Capacity:   cfg.RateLimitCapacity,
		RefillRate: cfg.RateLimitRefillRate,
		Enabled:    cfg.RateLimitEnabled,
	}
	rateLimiter := ratelimit.NewLineItemLimiter(rateLimiterConfig, metricsRegistry)

	// Initialize selector and configure rate limiting
	selector := &selectors.RuleBasedSelector{}
	selector.SetRateLimiter(rateLimiter)
	selector.SetLogger(logger)
	selector.SetProgrammaticBidTimeout(cfg.ProgrammaticBidTimeout)

	// Initialize CTR prediction client if enabled
	if cfg.CTROptimizationEnabled {
		ctrClient := optimization.NewCTRPredictionClient(
			cfg.CTRPredictorURL,
			cfg.CTRPredictorTimeout,
			cfg.CTRPredictorCacheTTL,
			logger,
			metricsRegistry,
		)

		// Start cache cleanup to prevent memory leaks
		ctrClient.StartCacheCleanup(10 * time.Minute) // Clean up expired entries every 10 minutes

		selector.SetCTRClient(ctrClient)
		logger.Info("CTR optimization enabled",
			zap.String("predictor_url", cfg.CTRPredictorURL),
			zap.Duration("timeout", cfg.CTRPredictorTimeout),
			zap.Duration("cache_ttl", cfg.CTRPredictorCacheTTL))
	}

	r := mux.NewRouter()
	// Pass the ad selector implementation here. Swap out RuleBasedSelector
	// for a custom one to change how ads are chosen.
	srvDeps := api.NewServer(logger, store, database, pg, analyticsSvc.DB, analyticsSvc, geoSvc, selector, cfg.DebugTrace, []byte(cfg.TokenSecret), cfg.TokenTTL, adDataStore, metricsRegistry, cfg)
	srvDeps.UpdateCTR()
	r.HandleFunc("/ad", srvDeps.GetAdHandler).Methods("POST")
	r.HandleFunc("/impression", srvDeps.ImpressionHandler).Methods("GET")
	r.HandleFunc("/click", srvDeps.ClickHandler).Methods("GET")
	r.HandleFunc("/event", srvDeps.EventHandler).Methods("GET")
	r.HandleFunc("/report", srvDeps.ReportHandler).Methods("POST")
	r.HandleFunc("/health", srvDeps.HealthHandler).Methods("GET")
	r.HandleFunc("/reload", srvDeps.ReloadHandler).Methods("POST")
	r.HandleFunc("/test/bid", srvDeps.TestBidHandler).Methods("POST")

	// CRUD routes for admin UI
	crud := r.PathPrefix("/api").Subrouter()
	crud.HandleFunc("/publishers", srvDeps.ListPublishers).Methods("GET")
	crud.HandleFunc("/publishers", srvDeps.CreatePublisher).Methods("POST")
	crud.HandleFunc("/publishers/{id}", srvDeps.UpdatePublisher).Methods("PUT")
	crud.HandleFunc("/publishers/{id}", srvDeps.DeletePublisher).Methods("DELETE")

	crud.HandleFunc("/campaigns", srvDeps.ListCampaigns).Methods("GET")
	crud.HandleFunc("/campaigns", srvDeps.CreateCampaign).Methods("POST")
	crud.HandleFunc("/campaigns/{id}", srvDeps.UpdateCampaign).Methods("PUT")
	crud.HandleFunc("/campaigns/{id}", srvDeps.DeleteCampaign).Methods("DELETE")

	crud.HandleFunc("/placements", srvDeps.ListPlacements).Methods("GET")
	crud.HandleFunc("/placements", srvDeps.CreatePlacement).Methods("POST")
	crud.HandleFunc("/placements/{id}", srvDeps.UpdatePlacement).Methods("PUT")
	crud.HandleFunc("/placements/{id}", srvDeps.DeletePlacement).Methods("DELETE")

	crud.HandleFunc("/line_items", srvDeps.ListLineItems).Methods("GET")
	crud.HandleFunc("/line_items", srvDeps.CreateLineItem).Methods("POST")
	crud.HandleFunc("/line_items/{id}", srvDeps.UpdateLineItem).Methods("PUT")
	crud.HandleFunc("/line_items/{id}", srvDeps.DeleteLineItem).Methods("DELETE")

	crud.HandleFunc("/creatives", srvDeps.ListCreatives).Methods("GET")
	crud.HandleFunc("/creatives", srvDeps.CreateCreative).Methods("POST")
	crud.HandleFunc("/creatives/{id}", srvDeps.UpdateCreative).Methods("PUT")
	crud.HandleFunc("/creatives/{id}", srvDeps.DeleteCreative).Methods("DELETE")

	// Static file server for serving static assets like HTML, CSS, JS
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// metrics endpoint (includes rate limiting metrics)
	r.Handle("/metrics", promhttp.Handler())

	addr := ":" + cfg.Port

	readTimeout := cfg.ReadTimeout
	writeTimeout := cfg.WriteTimeout

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	logger.Info("Ad server running", zap.String("addr", addr))

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("listen: %w", err)
		}
	}()

	if cfg.ReloadInterval > 0 {
		ticker := time.NewTicker(cfg.ReloadInterval)
		go func() {
			for {
				select {
				case <-ticker.C:
					if err := srvDeps.Reload(); err != nil {
						logger.Error("auto reload", zap.Error(err))
					}
					srvDeps.UpdateCTR()
				case <-ctx.Done():
					ticker.Stop()
					return
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	return nil
}
