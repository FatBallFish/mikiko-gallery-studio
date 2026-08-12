package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
)

func TestStartWorkerMetricsServerServesAndStopsWithContext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := startWorkerMetricsServer(ctx, listener, observability.NewMetrics().Handler())
	response, err := http.Get("http://" + listener.Addr().String() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("metrics shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("metrics server did not stop with context")
	}
}

func TestStartConfiguredWorkerMetricsServerListensOnRuntimeAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var listenedAddress string
	done, address, err := startConfiguredWorkerMetricsServer(ctx, "127.0.0.1:19091", observability.NewMetrics().Handler(), func(network, address string) (net.Listener, error) {
		if network != "tcp" {
			t.Fatalf("listen network = %q, want tcp", network)
		}
		listenedAddress = address
		return net.Listen("tcp", "127.0.0.1:0")
	})
	if err != nil {
		t.Fatal(err)
	}
	if listenedAddress != "127.0.0.1:19091" || address == "" {
		t.Fatalf("configured address = %q, bound address = %q", listenedAddress, address)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("metrics shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("configured metrics server did not stop with context")
	}
}
