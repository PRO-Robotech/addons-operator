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
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultQPS = 20
const defaultBurst = 40
const cleanupInterval = 5 * time.Minute
const maxIdleDuration = 10 * time.Minute
const defaultRequestTimeout = 10 * time.Second
const healthBackoffInitial = 15 * time.Second
const healthBackoffMax = 15 * time.Minute

// CacheKey uniquely identifies a remote cluster client by the Secret that holds its kubeconfig.
type CacheKey struct {
	Namespace  string
	SecretName string
}

// String returns a human-readable representation of the cache key.
func (k CacheKey) String() string {
	return fmt.Sprintf("%s/%s", k.Namespace, k.SecretName)
}

type cachedClient struct {
	client          client.Client
	resourceVersion string
	lastUsed        time.Time
}

type healthState struct {
	consecutiveFailures int
	openUntil           time.Time
}

// Cache holds controller-runtime clients per (namespace, secretName) and
// tracks per-cluster failure backoff so that dead remotes don't block workers.
type Cache struct {
	mu          sync.Mutex
	clients     map[CacheKey]*cachedClient
	health      map[CacheKey]*healthState
	scheme      *runtime.Scheme
	buildClient func(kubeconfigData []byte) (client.Client, error)
	now         func() time.Time
	stopCh      chan struct{}
	stopped     bool
}

// NewCache creates a new remote client cache that uses the given scheme
// when constructing controller-runtime clients.
func NewCache(scheme *runtime.Scheme) *Cache {
	c := &Cache{
		clients: make(map[CacheKey]*cachedClient),
		health:  make(map[CacheKey]*healthState),
		scheme:  scheme,
		now:     time.Now,
		stopCh:  make(chan struct{}),
	}
	c.buildClient = c.defaultBuildClient

	return c
}

// GetOrCreate returns a cached client for the given key, or creates a new one.
// If the resourceVersion differs from the cached entry the client is re-created,
// which handles kubeconfig rotation transparently.
func (c *Cache) GetOrCreate(kubeconfigData []byte, key CacheKey, resourceVersion string) (client.Client, error) {
	c.mu.Lock()
	if entry, ok := c.clients[key]; ok && entry.resourceVersion == resourceVersion {
		entry.lastUsed = time.Now()
		cl := entry.client
		c.mu.Unlock()

		return cl, nil
	}
	c.mu.Unlock()

	// Build outside the lock — may involve TLS handshake.
	newClient, err := c.buildClient(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("building remote client for %s: %w", key, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.clients[key]; ok && entry.resourceVersion == resourceVersion {
		entry.lastUsed = time.Now()

		return entry.client, nil
	}

	c.clients[key] = &cachedClient{
		client:          newClient,
		resourceVersion: resourceVersion,
		lastUsed:        time.Now(),
	}

	return newClient, nil
}

// CheckHealth returns (true, 0) if the caller may call the remote cluster,
// or (false, waitFor) if the cluster is in backoff — requeue after waitFor.
func (c *Cache) CheckHealth(key CacheKey) (ok bool, waitFor time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, exists := c.health[key]
	if !exists {
		return true, 0
	}

	remaining := state.openUntil.Sub(c.now())
	if remaining <= 0 {
		return true, 0
	}

	return false, remaining
}

// RecordFailure doubles the backoff for the cluster, up to healthBackoffMax.
func (c *Cache) RecordFailure(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, exists := c.health[key]
	if !exists {
		state = &healthState{}
		c.health[key] = state
	}
	state.consecutiveFailures++
	state.openUntil = c.now().Add(computeHealthBackoff(state.consecutiveFailures))
}

// RecordSuccess clears the backoff for the cluster.
func (c *Cache) RecordSuccess(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.health, key)
}

// computeHealthBackoff: 15s, 30s, 1m, 2m, 4m, 8m, 15m (capped).
func computeHealthBackoff(failures int) time.Duration {
	if failures < 1 {
		return healthBackoffInitial
	}
	shift := failures - 1
	if shift > 30 {
		return healthBackoffMax
	}
	d := healthBackoffInitial << shift
	if d <= 0 || d > healthBackoffMax {
		return healthBackoffMax
	}

	return d
}

// Invalidate removes the cached client for the given key.
func (c *Cache) Invalidate(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.clients, key)
}

// Start launches a background goroutine that periodically evicts idle clients.
func (c *Cache) Start() {
	go c.cleanupLoop()
}

// Stop terminates the background cleanup goroutine. It is safe to call multiple times.
func (c *Cache) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.stopped = true
		close(c.stopCh)
	}
}

// Len returns the number of cached clients.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.clients)
}

func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.evictIdle()
		}
	}
}

func (c *Cache) evictIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.clients {
		if now.Sub(entry.lastUsed) > maxIdleDuration {
			delete(c.clients, key)
		}
	}
}

func (c *Cache) defaultBuildClient(kubeconfigData []byte) (client.Client, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}
	restConfig.QPS = defaultQPS
	restConfig.Burst = defaultBurst
	if restConfig.Timeout == 0 {
		restConfig.Timeout = defaultRequestTimeout
	}

	return client.New(restConfig, client.Options{Scheme: c.scheme})
}
