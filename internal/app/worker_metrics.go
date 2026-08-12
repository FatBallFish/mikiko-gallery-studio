package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

func startWorkerMetricsServer(ctx context.Context, listener net.Listener, handler http.Handler) <-chan error {
	done := make(chan error, 1)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	return done
}

func startConfiguredWorkerMetricsServer(
	ctx context.Context,
	address string,
	handler http.Handler,
	listen func(string, string) (net.Listener, error),
) (<-chan error, string, error) {
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", address)
	if err != nil {
		return nil, "", err
	}
	return startWorkerMetricsServer(ctx, listener, handler), listener.Addr().String(), nil
}
