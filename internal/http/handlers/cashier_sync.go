package handlers

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/pkg/errs"
	"golang.org/x/sync/singleflight"
)

const (
	cashierOrderSyncThrottle     = time.Second
	cashierOrderSyncQueryTimeout = 20 * time.Second
	cashierOrderSyncCacheLimit   = 1024
)

type cachedCashierOrderSync struct {
	outcome  cashierOrderSyncOutcome
	syncedAt time.Time
}

type cashierOrderSyncOutcome struct {
	result adminCashierOrderSyncResult
	err    *errs.Error
}

type cashierOrderSyncCoordinator struct {
	group singleflight.Group
	mu    sync.Mutex
	cache map[int64]cachedCashierOrderSync
	now   func() time.Time
}

func (c *cashierOrderSyncCoordinator) Do(ctx context.Context, orderID int64, query func(context.Context) (adminCashierOrderSyncResult, *errs.Error)) (adminCashierOrderSyncResult, *errs.Error) {
	if cached, ok := c.cached(orderID); ok {
		return cached.result, cached.err
	}

	resultCh := c.group.DoChan(strconv.FormatInt(orderID, 10), func() (any, error) {
		if cached, ok := c.cached(orderID); ok {
			return cached, nil
		}
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cashierOrderSyncQueryTimeout)
		defer cancel()
		result, queryErr := query(queryCtx)
		outcome := cashierOrderSyncOutcome{result: result, err: queryErr}
		c.store(orderID, outcome)
		return outcome, nil
	})

	select {
	case <-ctx.Done():
		return adminCashierOrderSyncResult{}, errs.New(http.StatusRequestTimeout, "REQUEST_CANCELLED", "request was cancelled")
	case result := <-resultCh:
		if result.Err != nil {
			return adminCashierOrderSyncResult{}, errs.Internal("failed to synchronize cashier order")
		}
		outcome, ok := result.Val.(cashierOrderSyncOutcome)
		if !ok {
			return adminCashierOrderSyncResult{}, errs.Internal("failed to synchronize cashier order")
		}
		return outcome.result, outcome.err
	}
}

func (c *cashierOrderSyncCoordinator) cached(orderID int64) (cashierOrderSyncOutcome, bool) {
	now := c.currentTime()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		return cashierOrderSyncOutcome{}, false
	}
	cached, ok := c.cache[orderID]
	if !ok || now.Sub(cached.syncedAt) >= cashierOrderSyncThrottle {
		delete(c.cache, orderID)
		return cashierOrderSyncOutcome{}, false
	}
	return cached.outcome, true
}

func (c *cashierOrderSyncCoordinator) store(orderID int64, outcome cashierOrderSyncOutcome) {
	now := c.currentTime()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = make(map[int64]cachedCashierOrderSync)
	}
	if _, exists := c.cache[orderID]; !exists && len(c.cache) >= cashierOrderSyncCacheLimit {
		c.evictLocked(now)
	}
	c.cache[orderID] = cachedCashierOrderSync{outcome: outcome, syncedAt: now}
	if len(c.cache) > cashierOrderSyncCacheLimit {
		c.evictOldestLocked()
	}
}

func (c *cashierOrderSyncCoordinator) evictLocked(now time.Time) {
	for orderID, cached := range c.cache {
		if now.Sub(cached.syncedAt) >= cashierOrderSyncThrottle {
			delete(c.cache, orderID)
		}
	}
	if len(c.cache) >= cashierOrderSyncCacheLimit {
		c.evictOldestLocked()
	}
}

func (c *cashierOrderSyncCoordinator) evictOldestLocked() {
	var oldestOrderID int64
	var oldestSyncedAt time.Time
	for orderID, cached := range c.cache {
		if oldestSyncedAt.IsZero() || cached.syncedAt.Before(oldestSyncedAt) {
			oldestOrderID = orderID
			oldestSyncedAt = cached.syncedAt
		}
	}
	if !oldestSyncedAt.IsZero() {
		delete(c.cache, oldestOrderID)
	}
}

func (c *cashierOrderSyncCoordinator) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
