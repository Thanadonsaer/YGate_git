//go:build windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"chpp/modbus-api-middleware/internal/license"
)

func runInteractiveServiceMenu(publicKey string) (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return true, err
	}
	cfg := serviceConfig{
		DatabasePath:         filepath.Join(filepath.Dir(exe), "middleware.db"),
		Listen:               "0.0.0.0:8081",
		LicenseFile:          filepath.Join(filepath.Dir(exe), "license.json"),
		CleanupRetentionDays: 30,
	}
	return true, serviceMenu(bufio.NewReader(os.Stdin), os.Stdout, cfg, publicKey)
}

func serviceMenu(reader *bufio.Reader, output io.Writer, cfg serviceConfig, publicKey string) error {
	for {
		fmt.Fprintf(output, "\nCHPP Middleware Service Manager v%s\n", version)
		fmt.Fprintln(output, "Run as Administrator to change the Windows Service.")
		fmt.Fprintln(output, "1. Install / Update Service")
		fmt.Fprintln(output, "2. Service Status")
		fmt.Fprintln(output, "3. Start Service")
		fmt.Fprintln(output, "4. Stop Service")
		fmt.Fprintln(output, "5. Restart Service")
		fmt.Fprintln(output, "6. Uninstall Service")
		fmt.Fprintln(output, "7. Open Web UI")
		fmt.Fprintln(output, "8. Show Machine ID")
		fmt.Fprintln(output, "9. Activate License")
		fmt.Fprintln(output, "0. Exit")
		fmt.Fprint(output, "Select: ")

		choice, err := readMenuLine(reader)
		if err != nil {
			return err
		}
		switch choice {
		case "0":
			return nil
		case "1":
			fmt.Fprintf(output, "Database: %s\nLicense: %s\nListen: %s\n", cfg.DatabasePath, cfg.LicenseFile, cfg.Listen)
			if _, err = license.CheckFile(cfg.LicenseFile, publicKey); err == nil {
				cfg.Action = "install"
				err = manageService(cfg)
			} else {
				err = fmt.Errorf("activate license before installing service: %w", err)
			}
		case "2":
			cfg.Action = "status"
			err = manageService(cfg)
		case "3":
			cfg.Action = "start"
			err = manageService(cfg)
		case "4":
			cfg.Action = "stop"
			err = manageService(cfg)
		case "5":
			cfg.Action = "restart"
			err = manageService(cfg)
		case "6":
			if confirmed, readErr := confirmMenu(reader, output, "Uninstall Windows Service? [y/N]: "); readErr != nil {
				return readErr
			} else if confirmed {
				cfg.Action = "uninstall"
				err = manageService(cfg)
			}
		case "7":
			err = openBrowser("http://127.0.0.1:8081/")
		case "8":
			fmt.Fprintln(output, license.MachineID())
		case "9":
			fmt.Fprint(output, "License token: ")
			var token string
			token, err = readMenuLine(reader)
			if err == nil {
				var status license.Status
				status, err = license.Activate(cfg.LicenseFile, token, publicKey)
				if err == nil {
					fmt.Fprintf(output, "License activated for %s\n", firstNonEmpty(status.Payload.Customer, "unknown customer"))
				}
			}
		default:
			fmt.Fprintln(output, "Invalid selection.")
		}
		if err != nil {
			fmt.Fprintf(output, "Error: %v\n", err)
		}
		if choice != "0" {
			fmt.Fprint(output, "Press Enter to continue...")
			if _, err = reader.ReadString('\n'); err != nil && err != io.EOF {
				return err
			}
		}
	}
}

func confirmMenu(reader *bufio.Reader, output io.Writer, prompt string) (bool, error) {
	fmt.Fprint(output, prompt)
	value, err := readMenuLine(reader)
	return strings.EqualFold(value, "y") || strings.EqualFold(value, "yes"), err
}

func readMenuLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err == io.EOF && value == "" {
		return "", io.EOF
	}
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
