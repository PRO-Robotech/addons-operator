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

const (
	defaultQPS      = 20
	defaultBurst    = 40
	cleanupInterval = 5 * time.Minute
	maxIdleDuration = 10 * time.Minute
)

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

// Cache is a thread-safe cache of controller-runtime clients for remote clusters.
// Clients are keyed by (namespace, secretName) and invalidated when the Secret's
// resourceVersion changes (indicating kubeconfig rotation).
type Cache struct {
	mu          sync.Mutex
	clients     map[CacheKey]*cachedClient
	scheme      *runtime.Scheme
	buildClient func(kubeconfigData []byte) (client.Client, error)
	stopCh      chan struct{}
	stopped     bool
}

// NewCache creates a new remote client cache that uses the given scheme
// when constructing controller-runtime clients.
func NewCache(scheme *runtime.Scheme) *Cache {
	c := &Cache{
		clients: make(map[CacheKey]*cachedClient),
		scheme:  scheme,
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

	// Double-check: another goroutine may have populated the entry while we were building.
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

	return client.New(restConfig, client.Options{Scheme: c.scheme})
}
