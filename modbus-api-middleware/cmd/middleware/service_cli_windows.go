//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func manageService(cfg serviceConfig) error {
	switch cfg.Action {
	case "install":
		return installService(cfg)
	case "status":
		query, err := runServiceCommand("query")
		if err != nil {
			return err
		}
		config, _ := runServiceCommand("qc")
		fmt.Println(strings.TrimSpace(query + "\n" + config))
		return nil
	case "start":
		return printServiceCommand("start")
	case "stop":
		return stopService()
	case "restart":
		if err := stopService(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not been started") {
			return err
		}
		return printServiceCommand("start")
	case "uninstall":
		_ = stopService()
		return printServiceCommand("delete")
	default:
		return fmt.Errorf("unknown service action %q; use install, status, start, stop, restart, or uninstall", cfg.Action)
	}
}

func installService(cfg serviceConfig) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cfg.DatabasePath, err = filepath.Abs(cfg.DatabasePath)
	if err != nil {
		return err
	}
	cfg.LicenseFile, err = filepath.Abs(cfg.LicenseFile)
	if err != nil {
		return err
	}
	binPath := serviceBinaryPath(exe, cfg)
	_, queryErr := runServiceCommand("query")
	if queryErr == nil {
		_ = stopService()
		if _, err = runServiceCommand("config", "binPath=", binPath, "start=", "auto", "DisplayName=", "CHPP Modbus API Middleware"); err != nil {
			return err
		}
	} else if _, err = runServiceCommand("create", "binPath=", binPath, "start=", "auto", "DisplayName=", "CHPP Modbus API Middleware"); err != nil {
		return err
	}
	_, _ = runServiceCommand("description", "Reads Modbus TCP devices and sends readings to CHPP API.")
	_, _ = runServiceCommand("failure", "reset=", "60", "actions=", "restart/5000/restart/5000")
	if err = printServiceCommand("start"); err != nil {
		return err
	}
	fmt.Println("service installed and started")
	return nil
}

func serviceBinaryPath(exe string, cfg serviceConfig) string {
	return fmt.Sprintf(`"%s" -db "%s" -listen %s -cleanup-retention-days %d -require-license -license-file "%s"`, exe, cfg.DatabasePath, cfg.Listen, cfg.CleanupRetentionDays, cfg.LicenseFile)
}

func stopService() error {
	out, err := runServiceCommand("stop")
	if strings.TrimSpace(out) != "" {
		fmt.Println(out)
	}
	if err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := runServiceCommand("query")
		if strings.Contains(status, "STOPPED") {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("service did not stop within 30 seconds")
}

func printServiceCommand(action string) error {
	out, err := runServiceCommand(action)
	if strings.TrimSpace(out) != "" {
		fmt.Println(out)
	}
	return err
}

func runServiceCommand(action string, args ...string) (string, error) {
	cmdArgs := append([]string{action, serviceName}, args...)
	cmd := exec.Command("sc.exe", cmdArgs...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	text := strings.TrimSpace(output.String())
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}
