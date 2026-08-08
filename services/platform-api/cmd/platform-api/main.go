package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata"

	"ygate/platform-api/internal/bootstrap"
	"ygate/platform-api/internal/config"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/envfile"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/httpapi"
	"ygate/platform-api/internal/ingestion"
	"ygate/platform-api/internal/notify"
	"ygate/platform-api/internal/retention"
)

var version = "dev"

func main() {
	if err := envfile.LoadDefault(); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(ctx, 30*time.Second)
	pool, err := database.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		cancelStartup()
		log.Fatal(err)
	}
	defer pool.Close()
	// Migrations have just been applied, so a database that was created or
	// copied to this machine still needs its first login. Warn instead of
	// exiting: an install that already has an admin never gets past the check,
	// and ingestion does not depend on it.
	if err = bootstrap.EnsureAdmin(startupCtx, pool); err != nil {
		log.Printf("bootstrap: %v", err)
	}
	cancelStartup()

	hub := gatewayhub.New()
	registryService := core.New(pool, hub).WithMiddlewarePatchDir(cfg.MiddlewarePatchDir).WithSiteLogoDir(cfg.SiteLogoDir).WithPlantImageDir(cfg.PlantImageDir).WithPublicBaseURL(cfg.PublicBaseURL)
	mailer := notify.NewMailer(cfg.SMTPAddr, cfg.SMTPFrom, cfg.SMTPUsername, cfg.SMTPPassword)
	ingestionService := ingestion.New(pool, mailer)

	// Bounds telemetry growth; see migration 000040. Started before the server
	// so the first sweep happens at boot, not an hour in.
	go retention.Run(ctx, pool, cfg.TelemetryRetention, cfg.TelemetryRetentionScan)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.New(version, pool.Ping, pool, cfg.SessionIdleTimeout, registryService, ingestionService, hub, cfg.AllowedOrigins...),
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("platform-api %s listening on %s", version, cfg.ListenAddr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
