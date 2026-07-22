//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/fatballfish/pic-gallery/internal/servicehost"
)

func main() {
	options, err := parseOptions()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		_, _ = fmt.Fprintln(os.Stderr, "pic-gallery-service-host must be started by the Windows Service Control Manager")
		os.Exit(1)
	}
	if err := svc.Run(options.ServiceName, &serviceHandler{options: options}); err != nil {
		os.Exit(1)
	}
}

func parseOptions() (servicehost.ChildOptions, error) {
	var options servicehost.ChildOptions
	flag.StringVar(&options.ServiceName, "service-name", "", "Windows service name")
	flag.StringVar(&options.WorkingDirectory, "working-directory", "", "child working directory")
	flag.StringVar(&options.Executable, "executable", "", "child executable")
	flag.StringVar(&options.LogDirectory, "log-directory", "", "child log directory")
	flag.IntVar(&options.RestartExitCode, "restart-exit-code", 75, "child exit code requesting an internal restart")
	flag.Parse()
	options.Arguments = flag.Args()
	options.RestartDelay = 2 * time.Second
	return options, servicehost.Validate(options)
}

type serviceHandler struct {
	options servicehost.ChildOptions
}

func (handler *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	statuses <- svc.Status{State: svc.StartPending}
	result := make(chan error, 1)
	go func() {
		defer func() {
			if recover() != nil {
				result <- fmt.Errorf("service child supervisor panicked")
			}
		}()
		result <- servicehost.RunChild(ctx, handler.options)
	}()
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	statuses <- running
	for {
		select {
		case err := <-result:
			if err != nil {
				return false, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- running
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case err := <-result:
					if err != nil {
						return false, 1
					}
					return false, 0
				case <-time.After(30 * time.Second):
					return false, 1
				}
			}
		}
	}
}
