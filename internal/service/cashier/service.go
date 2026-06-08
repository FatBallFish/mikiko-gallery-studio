package cashier

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/shopspring/decimal"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

var (
	ErrPaymentMethodUnavailable   = errors.New("payment method is unavailable")
	ErrPaymentProviderUnavailable = errors.New("payment provider instance is unavailable")
)

type Service struct {
	mu             sync.Mutex
	schedulerState map[string]int64
}

func NewService() *Service {
	return &Service{schedulerState: map[string]int64{}}
}

func NewServiceWithSchedulerState(state map[string]int64) *Service {
	cloned := make(map[string]int64, len(state))
	for key, value := range state {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		cloned[trimmedKey] = value
	}
	return &Service{schedulerState: cloned}
}

func (s *Service) SchedulerState() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned := make(map[string]int64, len(s.schedulerState))
	for key, value := range s.schedulerState {
		cloned[key] = value
	}
	return cloned
}

func (s *Service) ScheduleProviderInstance(ctx context.Context, method domaincashier.VisibleMethod, instances []domaincashier.ProviderInstance, amountCNY string) (domaincashier.ProviderInstance, error) {
	return s.ScheduleProviderInstanceWithDailyUsage(ctx, method, instances, amountCNY, nil)
}

func (s *Service) ScheduleProviderInstanceWithDailyUsage(_ context.Context, method domaincashier.VisibleMethod, instances []domaincashier.ProviderInstance, amountCNY string, dailyUsage map[int64]decimal.Decimal) (domaincashier.ProviderInstance, error) {
	if strings.TrimSpace(method.Method) == "" || !method.Enabled {
		return domaincashier.ProviderInstance{}, ErrPaymentMethodUnavailable
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return domaincashier.ProviderInstance{}, ErrPaymentProviderUnavailable
	}

	candidates := make([]domaincashier.ProviderInstance, 0, len(instances))
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		if strings.TrimSpace(method.SourceProviderType) != "" && !strings.EqualFold(instance.ProviderType, method.SourceProviderType) {
			continue
		}
		if !stringListContains(instance.SupportedMethods, method.Method) {
			continue
		}
		if !domaincashier.ProviderInstanceAmountAllowedWithDailyUsage(instance, amount, dailyUsage[instance.ID]) {
			continue
		}
		if instance.ProviderType != "mock" && instance.ConfigStatus != "configured" {
			continue
		}
		candidates = append(candidates, instance)
	}
	if len(candidates) == 0 {
		return domaincashier.ProviderInstance{}, ErrPaymentProviderUnavailable
	}
	if strings.EqualFold(method.SchedulerStrategy, "random") && len(candidates) > 1 {
		return domaincashier.RandomProviderInstance(candidates), nil
	}
	if strings.EqualFold(method.SchedulerStrategy, "round_robin") && len(candidates) > 1 {
		return s.nextRoundRobinProviderInstance(method, candidates), nil
	}
	return candidates[0], nil
}

func (s *Service) nextRoundRobinProviderInstance(method domaincashier.VisibleMethod, candidates []domaincashier.ProviderInstance) domaincashier.ProviderInstance {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := SchedulerStateKey(method)
	lastID := s.schedulerState[key]
	nextIndex := 0
	if lastID > 0 {
		for index, candidate := range candidates {
			if candidate.ID == lastID {
				nextIndex = (index + 1) % len(candidates)
				break
			}
		}
	}
	selected := candidates[nextIndex]
	s.schedulerState[key] = selected.ID
	return selected
}

func SchedulerStateKey(method domaincashier.VisibleMethod) string {
	return strings.ToLower(strings.TrimSpace(method.Method)) + ":" + strings.ToLower(strings.TrimSpace(method.SourceProviderType))
}

func stringListContains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
