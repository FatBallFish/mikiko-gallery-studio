package imagetask_test

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"

	imagetask "github.com/fatballfish/pic-gallery/internal/service/imagetask"
)

func TestRedisConcurrencyGateSharesLimitAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	clientOne := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientTwo := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientOne.Close()
		_ = clientTwo.Close()
	})

	gateOne := imagetask.NewRedisConcurrencyGate(clientOne, "test")
	gateTwo := imagetask.NewRedisConcurrencyGate(clientTwo, "test")
	resources := []imagetask.ConcurrencyResource{{Key: "model-account:7", Limit: 1}}

	releaseOne, err := gateOne.Acquire(context.Background(), resources, time.Second)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}

	acquired := make(chan func(), 1)
	errCh := make(chan error, 1)
	go func() {
		release, acquireErr := gateTwo.Acquire(context.Background(), resources, time.Second)
		if acquireErr != nil {
			errCh <- acquireErr
			return
		}
		acquired <- release
	}()

	select {
	case release := <-acquired:
		release()
		t.Fatal("second gate bypassed the shared Redis limit")
	case err := <-errCh:
		t.Fatalf("second gate failed while waiting: %v", err)
	case <-time.After(75 * time.Millisecond):
	}

	releaseOne()
	select {
	case release := <-acquired:
		release()
	case err := <-errCh:
		t.Fatalf("second gate failed after release: %v", err)
	case <-time.After(time.Second):
		t.Fatal("second gate did not acquire after release")
	}
}

func TestRedisConcurrencyGateAcquiresResourcesAtomically(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	modelGate := imagetask.NewRedisConcurrencyGate(client, "test")
	waitingGate := imagetask.NewRedisConcurrencyGate(client, "test")
	observerGate := imagetask.NewRedisConcurrencyGate(client, "test")
	model := imagetask.ConcurrencyResource{Key: "model-account:9", Limit: 1}
	user := imagetask.ConcurrencyResource{Key: "user:42", Limit: 1}

	releaseModel, err := modelGate.Acquire(context.Background(), []imagetask.ConcurrencyResource{model}, time.Second)
	if err != nil {
		t.Fatalf("acquire model lease: %v", err)
	}

	waiting := make(chan func(), 1)
	errCh := make(chan error, 1)
	go func() {
		release, acquireErr := waitingGate.Acquire(context.Background(), []imagetask.ConcurrencyResource{user, model}, time.Second)
		if acquireErr != nil {
			errCh <- acquireErr
			return
		}
		waiting <- release
	}()

	select {
	case release := <-waiting:
		release()
		t.Fatal("combined lease bypassed the occupied model resource")
	case err := <-errCh:
		t.Fatalf("combined lease failed while waiting: %v", err)
	case <-time.After(75 * time.Millisecond):
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	releaseUser, err := observerGate.Acquire(ctx, []imagetask.ConcurrencyResource{user}, time.Second)
	if err != nil {
		t.Fatalf("waiting combined lease held the user resource prematurely: %v", err)
	}
	releaseUser()

	releaseModel()
	select {
	case release := <-waiting:
		release()
	case err := <-errCh:
		t.Fatalf("combined lease failed after model release: %v", err)
	case <-time.After(time.Second):
		t.Fatal("combined lease did not acquire after model release")
	}
}

func TestRedisConcurrencyGateRecoversExpiredLease(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	gateOne := imagetask.NewRedisConcurrencyGate(client, "test")
	gateTwo := imagetask.NewRedisConcurrencyGate(client, "test")
	resources := []imagetask.ConcurrencyResource{{Key: "user:88", Limit: 1}}

	if _, err := gateOne.Acquire(context.Background(), resources, 50*time.Millisecond); err != nil {
		t.Fatalf("acquire abandoned lease: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := gateTwo.Acquire(ctx, resources, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire after lease expiry: %v", err)
	}
	release()
}

func TestRedisConcurrencyGateWaitHonorsContextCancellation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	gate := imagetask.NewRedisConcurrencyGate(client, "test")
	resources := []imagetask.ConcurrencyResource{{Key: "user:99", Limit: 1}}
	release, err := gate.Acquire(context.Background(), resources, time.Second)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := gate.Acquire(ctx, resources, time.Second); err != context.DeadlineExceeded {
		t.Fatalf("expected context deadline while waiting, got %v", err)
	}
}
