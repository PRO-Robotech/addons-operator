package main

import (
	"fmt"
	"time"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// defaultSyncRetryFromFlags собирает политику повтора синхронизации по умолчанию (K8S-1228).
// Вызывается только при limit != 0 (0 — умолчания нет). Предел обязателен: бесконечного повтора
// (-1 у Argo CD) нет.
func defaultSyncRetryFromFlags(limit int64, duration string, factor int64, maxDuration string) (*addonsv1alpha1.RetryStrategy, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("default-sync-retry-limit %d: допустимо 0 (выключено) или 1..100", limit)
	}
	if factor < 1 {
		return nil, fmt.Errorf("default-sync-retry-backoff-factor %d: допустимо от 1", factor)
	}
	for name, d := range map[string]string{"duration": duration, "max-duration": maxDuration} {
		if _, err := time.ParseDuration(d); err != nil {
			return nil, fmt.Errorf("default-sync-retry-backoff-%s %q: %w", name, d, err)
		}
	}
	f := factor

	return &addonsv1alpha1.RetryStrategy{
		Limit:   limit,
		Backoff: &addonsv1alpha1.Backoff{Duration: duration, Factor: &f, MaxDuration: maxDuration},
	}, nil
}
