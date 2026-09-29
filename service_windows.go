//go:build windows

package main

import (
	"log"

	"golang.org/x/sys/windows/svc"
)

const serviceName = "plmWebTail"

// isService returns true if the current process is running as a Windows service
func isService() bool {
	b, err := svc.IsWindowsService()
	if err != nil {
		log.Printf("Failed to determine service status: %v", err)
		return false
	}
	return b
}

// runService registers and runs the Windows service
func runService() {
	log.Printf("Starting %s as a Windows service", serviceName)
	err := svc.Run(serviceName, &windowsService{})
	if err != nil {
		log.Fatalf("Service %s run failed: %v", serviceName, err)
	}
}

type windowsService struct{}

// Execute implements svc.Handler
func (s *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// Start the HTTP server in a goroutine
	go func() {
		runServer()
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	log.Printf("Service %s is now running", serviceName)

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				log.Printf("Service %s is stopping", serviceName)
				changes <- svc.Status{State: svc.StopPending}
				return false, 0
			default:
				log.Printf("Service %s: unexpected control request #%d", serviceName, c)
			}
		}
	}
}
