package cashier

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type CustomAmountConfig struct {
	Enabled      bool   `json:"enabled"`
	MinAmountCNY string `json:"min_amount_cny"`
	MaxAmountCNY string `json:"max_amount_cny"`
	CNYPerPoint  string `json:"cny_per_point"`
}

type VisibleMethod struct {
	Method             string `json:"method"`
	Label              string `json:"label"`
	Enabled            bool   `json:"enabled"`
	SourceProviderType string `json:"source_provider_type,omitempty"`
	SchedulerStrategy  string `json:"scheduler_strategy,omitempty"`
	DisplayOrder       int    `json:"display_order"`
	Description        string `json:"description,omitempty"`
}

type ProviderInstance struct {
	ID               int64          `json:"id"`
	ProviderType     string         `json:"provider_type"`
	Name             string         `json:"name"`
	Enabled          bool           `json:"enabled"`
	SupportedMethods []string       `json:"supported_methods"`
	SortOrder        int            `json:"sort_order"`
	SchedulerWeight  int            `json:"scheduler_weight"`
	Limits           map[string]any `json:"limits,omitempty"`
	Config           map[string]any `json:"config,omitempty"`
	ConfigStatus     string         `json:"config_status,omitempty"`
	LastError        string         `json:"last_error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func RandomProviderInstance(candidates []ProviderInstance) ProviderInstance {
	return RandomProviderInstanceWithReader(rand.Reader, candidates)
}

func RandomProviderInstanceWithReader(reader io.Reader, candidates []ProviderInstance) ProviderInstance {
	if len(candidates) == 0 {
		return ProviderInstance{}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	max := big.NewInt(int64(len(candidates)))
	var buf [8]byte
	if _, err := io.ReadFull(reader, buf[:]); err == nil {
		value := new(big.Int).SetUint64(binary.BigEndian.Uint64(buf[:]))
		index := new(big.Int).Mod(value, max).Int64()
		return candidates[index]
	}
	return candidates[0]
}

func ProviderInstanceAmountAllowed(instance ProviderInstance, amount decimal.Decimal) bool {
	minRaw := strings.TrimSpace(rawString(instance.Limits["min_amount_cny"]))
	if minRaw != "" {
		min, err := decimal.NewFromString(minRaw)
		if err != nil || amount.LessThan(min) {
			return false
		}
	}
	maxRaw := strings.TrimSpace(rawString(instance.Limits["max_amount_cny"]))
	if maxRaw != "" {
		max, err := decimal.NewFromString(maxRaw)
		if err != nil || amount.GreaterThan(max) {
			return false
		}
	}
	return true
}

func rawString(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}
