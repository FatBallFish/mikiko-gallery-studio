package cashier

import (
	"fmt"
	"io"
)

const maxCashierProviderResponseBytes = 1 << 20

func readCashierProviderResponse(body io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(body, maxCashierProviderResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxCashierProviderResponseBytes {
		return nil, fmt.Errorf("payment provider response exceeds %d bytes", maxCashierProviderResponseBytes)
	}
	return content, nil
}
