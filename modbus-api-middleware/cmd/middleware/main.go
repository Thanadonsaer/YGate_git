package main

import (
	"chpp/modbus-api-middleware/internal/app"
	"chpp/modbus-api-middleware/internal/delivery"
	"chpp/modbus-api-middleware/internal/license"
	"chpp/modbus-api-middleware/internal/modbus"
	"chpp/modbus-api-middleware/internal/store"
	webui "chpp/modbus-api-middleware/internal/web"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var (
	version          = "0.2.6"
	commit           = "dev"
	licensePublicKey = ""
	serviceMode      bool
)

func main() {
	service, err := runMaybeService(run)
	if err != nil {
		log.Fatal(err)
	}
	if service {
		return
	}
	if len(os.Args) == 1 {
		publicKey := firstNonEmpty(licensePublicKey, os.Getenv("CHPP_LICENSE_PUBLIC_KEY"))
		handled, err := runInteractiveServiceMenu(publicKey)
		if err != nil {
			log.Fatal(err)
		}
		if handled {
			return
		}
	}
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	dbPath := flag.String("db", "middleware.db", "SQLite path")
	listen := flag.String("listen", "127.0.0.1:8081", "web listen address")
	endpoint := flag.String("endpoint", "", "CHPP readings endpoint")
	apiKey := flag.String("api-key", "", "gateway API key")
	gateway := flag.String("gateway-id", "", "local gateway identity")
	runServer := flag.Bool("run", false, "run polling and the web server in console mode")
	serviceAction := flag.String("service", "", "manage Windows Service: install, status, start, stop, restart, uninstall")
	configureOnly := flag.Bool("configure-only", false, "save gateway flags and exit without polling or starting the web server")
	cleanupRetentionDays := flag.Int("cleanup-retention-days", 30, "delete delivered/dead-letter queue and poll logs older than this many days; 0 disables cleanup")
	cleanupIntervalHours := flag.Int("cleanup-interval-hours", 24, "cleanup interval in hours")
	showVersion := flag.Bool("version", false, "print version and exit")
	licenseFile := flag.String("license-file", "license.json", "license activation file")
	requireLicense := flag.Bool("require-license", false, "require a valid license before starting")
	activateLicense := flag.String("activate-license", "", "activate an offline license token and save license-file")
	licenseStatus := flag.Bool("license-status", false, "check license-file and exit")
	machineID := flag.Bool("machine-id", false, "print machine id for license activation and exit")
	gui := flag.Bool("gui", false, "open web GUI in browser; implies -run")
	flag.Parse()
	if *showVersion {
		fmt.Printf("chpp-middleware %s (%s)\n", version, commit)
		return nil
	}
	publicKey := firstNonEmpty(licensePublicKey, os.Getenv("CHPP_LICENSE_PUBLIC_KEY"))
	if *machineID {
		fmt.Println(license.MachineID())
		return nil
	}
	if *activateLicense != "" {
		status, err := license.Activate(*licenseFile, *activateLicense, publicKey)
		if err != nil {
			return err
		}
		fmt.Printf("license activated for %s (%s)\n", firstNonEmpty(status.Payload.Customer, "unknown customer"), status.MachineID)
		return nil
	}
	if *licenseStatus {
		status, err := license.CheckFile(*licenseFile, publicKey)
		if err != nil {
			return err
		}
		fmt.Printf("license valid for %s (%s)\n", firstNonEmpty(status.Payload.Customer, "unknown customer"), status.MachineID)
		return nil
	}
	action := strings.ToLower(strings.TrimSpace(*serviceAction))
	if action != "" {
		if action == "install" {
			if _, err := license.CheckFile(*licenseFile, publicKey); err != nil {
				return fmt.Errorf("activate license before installing service: %w", err)
			}
		}
		return manageService(serviceConfig{
			Action:               action,
			DatabasePath:         *dbPath,
			Listen:               *listen,
			LicenseFile:          *licenseFile,
			CleanupRetentionDays: *cleanupRetentionDays,
		})
	}
	if !serviceMode && !*runServer && !*gui && !*configureOnly {
		fmt.Println("middleware not started; install the service with -service install or use -run for console mode")
		flag.PrintDefaults()
		return nil
	}
	if *requireLicense {
		if _, err := license.CheckFile(*licenseFile, publicKey); err != nil {
			return fmt.Errorf("license required: %w", err)
		}
	}

	st, err := store.OpenNormalized(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	cfg, err := st.GatewayConfig()
	if err != nil {
		return err
	}
	if *gateway != "" {
		cfg.GatewayID = *gateway
	}
	if *endpoint != "" {
		cfg.Endpoint = *endpoint
	}
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}
	if *endpoint != "" || *apiKey != "" {
		cfg.APIPollingEnabled = true
	}
	if *gateway != "" || *endpoint != "" || *apiKey != "" {
		if _, err = st.SaveGatewayConfig(cfg); err != nil {
			return err
		}
	}
	if *configureOnly {
		return nil
	}

	svc := &app.Service{Store: st, Client: &modbus.Client{Timeout: 3 * time.Second}}
	go runCleanup(ctx, st, *cleanupRetentionDays, *cleanupIntervalHours)

	worker := &delivery.Worker{Store: st, BatchSize: 20, Client: &http.Client{Timeout: 10 * time.Second}, BeforeSend: func() error {
		return svc.PollEnabledConnections(cfg.GatewayID, log.Printf)
	}}
	go runDelivery(ctx, worker)

	server := &webui.Server{Store: st, App: svc, GatewayID: cfg.GatewayID, Version: version, CanApplyUpdate: serviceMode || os.Getenv("INVOCATION_ID") != "", LicensePublicKey: publicKey, LicenseFile: *licenseFile}
	httpServer := &http.Server{Addr: *listen, Handler: server.FullHandler()}
	errCh := make(chan error, 1)
	if *gui {
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = openBrowser(browserURL(*listen))
		}()
	}
	go func() {
		log.Printf("listening on %s", *listen)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runDelivery(ctx context.Context, worker *delivery.Worker) {
	for {
		if _, err := worker.SendOnce(); err != nil {
			log.Printf("delivery: %v", err)
		}
		timer := time.NewTimer(worker.Interval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func runCleanup(ctx context.Context, st *store.Store, retentionDays, intervalHours int) {
	if retentionDays <= 0 {
		return
	}
	if intervalHours < 1 {
		intervalHours = 24
	}
	cleanup := func() {
		deleted, err := st.CleanupOld(time.Duration(retentionDays) * 24 * time.Hour)
		if err != nil {
			log.Printf("cleanup: %v", err)
		} else if deleted > 0 {
			log.Printf("cleanup: deleted %d old rows", deleted)
		}
	}
	cleanup()
	ticker := time.NewTicker(time.Duration(intervalHours) * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func browserURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://" + listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
