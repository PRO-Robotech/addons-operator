/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package remoteclient

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestCache() *Cache {
	scheme := runtime.NewScheme()
	c := NewCache(scheme)
	c.buildClient = func(_ []byte) (client.Client, error) {
		return fake.NewClientBuilder().WithScheme(scheme).Build(), nil
	}

	return c
}

func TestNewCache(t *testing.T) {
	c := newTestCache()
	require.NotNil(t, c)
	assert.Equal(t, 0, c.Len())
}

func TestGetOrCreate_NewClient(t *testing.T) {
	c := newTestCache()
	key := CacheKey{Namespace: "ns", SecretName: "secret"}

	cl, err := c.GetOrCreate([]byte("kubeconfig"), key, "v1")

	require.NoError(t, err)
	require.NotNil(t, cl)
	assert.Equal(t, 1, c.Len())
}

func TestGetOrCreate_CachedClient(t *testing.T) {
	c := newTestCache()
	key := CacheKey{Namespace: "ns", SecretName: "secret"}

	cl1, err := c.GetOrCreate([]byte("kubeconfig"), key, "v1")
	require.NoError(t, err)

	cl2, err := c.GetOrCreate([]byte("kubeconfig"), key, "v1")
	require.NoError(t, err)

	// Same pointer — client was reused from cache.
	assert.Same(t, cl1, cl2)
	assert.Equal(t, 1, c.Len())
}

func TestGetOrCreate_ResourceVersionChanged(t *testing.T) {
	c := newTestCache()
	key := CacheKey{Namespace: "ns", SecretName: "secret"}

	cl1, err := c.GetOrCreate([]byte("kubeconfig"), key, "v1")
	require.NoError(t, err)

	cl2, err := c.GetOrCreate([]byte("kubeconfig-new"), key, "v2")
	require.NoError(t, err)

	assert.NotSame(t, cl1, cl2)
	assert.Equal(t, 1, c.Len())
}

func TestGetOrCreate_DifferentKeys(t *testing.T) {
	c := newTestCache()
	key1 := CacheKey{Namespace: "ns1", SecretName: "secret1"}
	key2 := CacheKey{Namespace: "ns2", SecretName: "secret2"}

	cl1, err := c.GetOrCreate([]byte("kc1"), key1, "v1")
	require.NoError(t, err)

	cl2, err := c.GetOrCreate([]byte("kc2"), key2, "v1")
	require.NoError(t, err)

	assert.NotSame(t, cl1, cl2)
	assert.Equal(t, 2, c.Len())
}

func TestGetOrCreate_BuildError(t *testing.T) {
	c := newTestCache()
	c.buildClient = func(_ []byte) (client.Client, error) {
		return nil, errors.New("connection refused")
	}
	key := CacheKey{Namespace: "ns", SecretName: "secret"}

	cl, err := c.GetOrCreate([]byte("bad"), key, "v1")

	require.Error(t, err)
	assert.Nil(t, cl)
	assert.Equal(t, 0, c.Len())
}

func TestInvalidate(t *testing.T) {
	c := newTestCache()
	key := CacheKey{Namespace: "ns", SecretName: "secret"}

	_, err := c.GetOrCreate([]byte("kubeconfig"), key, "v1")
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len())

	c.Invalidate(key)
	assert.Equal(t, 0, c.Len())
}

func TestInvalidate_NonExistent(t *testing.T) {
	c := newTestCache()
	key := CacheKey{Namespace: "ns", SecretName: "nonexistent"}

	// Should not panic.
	c.Invalidate(key)
	assert.Equal(t, 0, c.Len())
}

func TestEvictIdle(t *testing.T) {
	c := newTestCache()

	freshKey := CacheKey{Namespace: "ns", SecretName: "fresh"}
	staleKey := CacheKey{Namespace: "ns", SecretName: "stale"}

	_, err := c.GetOrCreate([]byte("kc"), freshKey, "v1")
	require.NoError(t, err)

	_, err = c.GetOrCreate([]byte("kc"), staleKey, "v1")
	require.NoError(t, err)
	assert.Equal(t, 2, c.Len())

	// Backdate the stale entry.
	c.mu.Lock()
	c.clients[staleKey].lastUsed = time.Now().Add(-maxIdleDuration - time.Minute)
	c.mu.Unlock()

	c.evictIdle()

	assert.Equal(t, 1, c.Len())

	// Verify the fresh entry survived.
	c.mu.Lock()
	_, hasFresh := c.clients[freshKey]
	_, hasStale := c.clients[staleKey]
	c.mu.Unlock()
	assert.True(t, hasFresh)
	assert.False(t, hasStale)
}

func TestStartStop(t *testing.T) {
	c := newTestCache()

	// Start and stop should not panic.
	c.Start()
	c.Stop()

	// Double stop should not panic.
	c.Stop()
}

func TestConcurrentAccess(t *testing.T) {
	c := newTestCache()
	var wg sync.WaitGroup

	// 100 goroutines performing GetOrCreate.
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := CacheKey{Namespace: "ns", SecretName: fmt.Sprintf("secret-%d", n%10)}
			_, _ = c.GetOrCreate([]byte("kc"), key, "v1")
		}(i)
	}

	// 50 goroutines performing Invalidate.
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := CacheKey{Namespace: "ns", SecretName: fmt.Sprintf("secret-%d", n%10)}
			c.Invalidate(key)
		}(i)
	}

	wg.Wait()
}

func TestCacheKey_String(t *testing.T) {
	key := CacheKey{Namespace: "my-namespace", SecretName: "my-secret"}
	assert.Equal(t, "my-namespace/my-secret", key.String())
}

func TestCheckHealth_NoHistoryIsHealthy(t *testing.T) {
	c := newTestCache()
	ok, waitFor := c.CheckHealth(CacheKey{Namespace: "ns", SecretName: "s"})
	assert.True(t, ok)
	assert.Equal(t, time.Duration(0), waitFor)
}

func TestRecordFailure_BacksOffExponentially(t *testing.T) {
	c := newTestCache()
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	key := CacheKey{Namespace: "ns", SecretName: "s"}

	expectations := []time.Duration{
		15 * time.Second,
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		15 * time.Minute, // would be 16m, capped at 15m
		15 * time.Minute, // stays capped
	}

	for i, want := range expectations {
		c.RecordFailure(key)

		ok, waitFor := c.CheckHealth(key)
		assert.False(t, ok, "attempt %d should be gated", i+1)
		assert.Equal(t, want, waitFor, "attempt %d wrong backoff", i+1)
	}
}

func TestRecordSuccess_ClearsBackoff(t *testing.T) {
	c := newTestCache()
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	key := CacheKey{Namespace: "ns", SecretName: "s"}
	c.RecordFailure(key)
	c.RecordFailure(key)

	ok, _ := c.CheckHealth(key)
	require.False(t, ok, "should be gated after failures")

	c.RecordSuccess(key)

	ok, waitFor := c.CheckHealth(key)
	assert.True(t, ok, "success must clear backoff")
	assert.Equal(t, time.Duration(0), waitFor)

	// A fresh failure starts from the initial backoff, not where the series left off.
	c.RecordFailure(key)
	_, waitFor = c.CheckHealth(key)
	assert.Equal(t, healthBackoffInitial, waitFor, "counter must reset after success")
}

func TestCheckHealth_ResumesAfterBackoffElapses(t *testing.T) {
	c := newTestCache()
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	key := CacheKey{Namespace: "ns", SecretName: "s"}
	c.RecordFailure(key)

	// Right before backoff ends — still gated.
	now = now.Add(healthBackoffInitial - time.Nanosecond)
	ok, _ := c.CheckHealth(key)
	assert.False(t, ok)

	// After backoff elapses — callers may attempt again.
	now = now.Add(2 * time.Nanosecond)
	ok, waitFor := c.CheckHealth(key)
	assert.True(t, ok)
	assert.Equal(t, time.Duration(0), waitFor)
}

func TestComputeHealthBackoff_CapsAtMax(t *testing.T) {
	assert.Equal(t, healthBackoffInitial, computeHealthBackoff(1))
	assert.Equal(t, 2*healthBackoffInitial, computeHealthBackoff(2))
	assert.Equal(t, healthBackoffMax, computeHealthBackoff(7))  // would overflow past cap
	assert.Equal(t, healthBackoffMax, computeHealthBackoff(50)) // obvious cap
	assert.Equal(t, healthBackoffMax, computeHealthBackoff(1000))
}
