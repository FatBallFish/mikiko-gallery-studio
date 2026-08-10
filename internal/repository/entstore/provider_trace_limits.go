package entstore

import "errors"

const (
	maxProviderTraceSemanticBytes = 8 << 20
	maxProviderTraceAttempts      = 10_000
)

var errProviderTraceExceedsLimits = errors.New("provider trace exceeds limits")
