//go:build !windows

package main

import "fmt"

func manageService(cfg serviceConfig) error {
	return fmt.Errorf("service action %q is available on Windows only; use systemd scripts on Linux", cfg.Action)
}
