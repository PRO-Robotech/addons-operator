package main

import "testing"

// K8S-1228: разбор флагов политики повтора по умолчанию.
func TestDefaultSyncRetryFromFlags(t *testing.T) {
	r, err := defaultSyncRetryFromFlags(12, "30s", 2, "10m")
	if err != nil || r == nil || r.Limit != 12 || r.Backoff == nil || *r.Backoff.Factor != 2 || r.Backoff.MaxDuration != "10m" {
		t.Fatalf("limit 12: неверно %+v, %v", r, err)
	}
	for _, c := range []struct {
		limit, factor int64
		d, md         string
	}{{0, 2, "30s", "10m"}, {-1, 2, "30s", "10m"}, {101, 2, "30s", "10m"}, {5, 0, "30s", "10m"}, {5, 2, "x", "10m"}, {5, 2, "30s", "y"}} {
		if _, err := defaultSyncRetryFromFlags(c.limit, c.d, c.factor, c.md); err == nil {
			t.Errorf("ждали ошибку для %+v", c)
		}
	}
}
