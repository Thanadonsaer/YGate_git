//go:build windows

package main

import (
	"context"
	"time"

	"golang.org/x/sys/windows/svc"

	"chpp/modbus-api-middleware/internal/updatebridge"
)

const serviceName = "chpp-middleware"

func runMaybeService(dbPath string) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, nil
	}
	return true, svc.Run(serviceName, serviceHandler{dbPath: dbPath})
}

type serviceHandler struct{ dbPath string }

func (h serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	status <- svc.Status{State: svc.StartPending}
	go func() { done <- (&updatebridge.Bridge{DBPath: h.dbPath, Version: version}).Run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
					return false, 0
				case <-time.After(15 * time.Second):
					return false, 0
				}
			}
		case err := <-done:
			if err != nil {
				return false, 1
			}
			return false, 0
		}
	}
}
