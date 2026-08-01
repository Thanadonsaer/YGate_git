package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ygate/platform-api/internal/config"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/envfile"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/httpapi"
	"ygate/platform-api/internal/ingestion"
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
	cancelStartup()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	hub := gatewayhub.New()
	registryService := core.New(pool, hub)
	ingestionService := ingestion.New(pool)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.New(version, pool.Ping, pool, registryService, ingestionService, hub, cfg.AllowedOrigins...),
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
