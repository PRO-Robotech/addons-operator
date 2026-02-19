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
