package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/config"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/envfile"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/httpapi"
	"ygate/platform-api/internal/ingestion"
	"ygate/platform-api/internal/notification"
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

	authService := auth.New(pool, cfg.SessionIdleTimeout, cfg.SessionAbsoluteTimeout)
	var resetNotifier auth.ResetNotifier
	if cfg.SMTPAddr != "" {
		notifier, notifierErr := notification.NewSMTPResetNotifier(
			cfg.SMTPAddr, cfg.SMTPFrom, cfg.SMTPUsername, cfg.SMTPPassword, cfg.PasswordResetURL,
		)
		if notifierErr != nil {
			log.Fatal(notifierErr)
		}
		resetNotifier = notifier.Notify
	}
	authService.ConfigurePasswordRecovery(cfg.PasswordResetTTL, resetNotifier)
	hub := gatewayhub.New()
	registryService := core.New(pool, hub)
	ingestionService := ingestion.New(pool)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.New(version, pool.Ping, authService, registryService, ingestionService, hub, cfg.CookieSecure, cfg.AllowedOrigins...),
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
