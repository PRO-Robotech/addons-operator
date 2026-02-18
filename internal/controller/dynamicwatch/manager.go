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

package dynamicwatch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// WatchManager manages dynamic watches for arbitrary GVKs.
type WatchManager interface {
	// EnsureWatch creates or increments refCount for a watch.
	// Returns nil if CRD is not available (graceful degradation).
	EnsureWatch(ctx context.Context, gvk schema.GroupVersionKind) error

	// ReleaseWatch decrements refCount for a watch.
	ReleaseWatch(gvk schema.GroupVersionKind)

	// RetryPendingWatch attempts to activate a pending watch.
	// Returns nil if watch was activated or wasn't pending.
	// Returns error if CRD is still not available.
	RetryPendingWatch(ctx context.Context, gvk schema.GroupVersionKind) error

	// GetPendingGVKs returns GVKs that couldn't be watched (CRD not available).
	GetPendingGVKs() []schema.GroupVersionKind

	// GetActiveGVKs returns currently active GVKs.
	GetActiveGVKs() []schema.GroupVersionKind

	// GetWatchCount returns the number of active watches.
	GetWatchCount() int
}

// watchEntry tracks a single GVK watch.
type watchEntry struct {
	gvk       schema.GroupVersionKind
	refCount  int
	active    bool
	pending   bool
	startedAt time.Time
}

// UnstructuredMapFunc is a typed map function for Unstructured objects.
type UnstructuredMapFunc = handler.TypedMapFunc[*unstructured.Unstructured, reconcile.Request]

// manager implements WatchManager.
type manager struct {
	cache      cache.Cache
	controller controller.Controller
	mapFunc    UnstructuredMapFunc

	watches map[schema.GroupVersionKind]*watchEntry
	mu      sync.RWMutex

	logger logr.Logger
}

// NewWatchManager creates a new dynamic watch manager.
func NewWatchManager(
	c cache.Cache,
	ctrl controller.Controller,
	mapFunc UnstructuredMapFunc,
	logger logr.Logger,
) WatchManager {
	return &manager{
		cache:      c,
		controller: ctrl,
		mapFunc:    mapFunc,
		watches:    make(map[schema.GroupVersionKind]*watchEntry),
		logger:     logger.WithName("dynamicwatch"),
	}
}

// EnsureWatch creates or increments refCount for a watch.
func (m *manager) EnsureWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if watch already exists
	if entry, exists := m.watches[gvk]; exists {
		entry.refCount++
		m.logger.V(1).Info("incremented watch refCount",
			"gvk", gvk.String(),
			"refCount", entry.refCount)
		return nil
	}

	// Create new watch
	m.logger.Info("creating new watch", "gvk", gvk.String())

	// Create Unstructured object for this GVK
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	// Create typed handler for unstructured objects
	typedHandler := handler.TypedEnqueueRequestsFromMapFunc(m.mapFunc)

	// Create source.Kind for the watch
	kindSource := source.Kind(m.cache, u, typedHandler)

	// Start the watch
	if err := m.controller.Watch(kindSource); err != nil {
		// Check if CRD is not installed
		if isNotFoundError(err) {
			m.watches[gvk] = &watchEntry{
				gvk:      gvk,
				refCount: 1,
				pending:  true,
			}
			m.logger.Info("watch pending - CRD not available",
				"gvk", gvk.String())
			return nil // Graceful degradation
		}
		return fmt.Errorf("start watch for %s: %w", gvk.String(), err)
	}

	m.watches[gvk] = &watchEntry{
		gvk:       gvk,
		refCount:  1,
		active:    true,
		startedAt: time.Now(),
	}

	m.logger.Info("watch started", "gvk", gvk.String())
	return nil
}

// ReleaseWatch decrements refCount for a watch.
func (m *manager) ReleaseWatch(gvk schema.GroupVersionKind) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.watches[gvk]
	if !exists {
		m.logger.V(1).Info("release called for unknown watch", "gvk", gvk.String())
		return
	}

	entry.refCount--
	m.logger.V(1).Info("decremented watch refCount",
		"gvk", gvk.String(),
		"refCount", entry.refCount)

	if entry.refCount <= 0 {
		delete(m.watches, gvk)
		m.logger.Info("watch removed from tracking", "gvk", gvk.String())
	}
}

// GetPendingGVKs returns GVKs that couldn't be watched.
func (m *manager) GetPendingGVKs() []schema.GroupVersionKind {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []schema.GroupVersionKind
	for gvk, entry := range m.watches {
		if entry.pending {
			pending = append(pending, gvk)
		}
	}
	return pending
}

// GetActiveGVKs returns currently active GVKs.
func (m *manager) GetActiveGVKs() []schema.GroupVersionKind {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []schema.GroupVersionKind
	for gvk, entry := range m.watches {
		if entry.active {
			active = append(active, gvk)
		}
	}
	return active
}

// GetWatchCount returns the number of watches (for testing).
func (m *manager) GetWatchCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.watches)
}

// RetryPendingWatch attempts to activate a pending watch.
func (m *manager) RetryPendingWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.watches[gvk]
	if !exists || !entry.pending {
		return nil // Nothing to retry
	}

	// Try to create watch again
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	typedHandler := handler.TypedEnqueueRequestsFromMapFunc(m.mapFunc)
	kindSource := source.Kind(m.cache, u, typedHandler)

	if err := m.controller.Watch(kindSource); err != nil {
		if isNotFoundError(err) {
			return fmt.Errorf("CRD still not available for %s", gvk.String())
		}
		return fmt.Errorf("start watch for %s: %w", gvk.String(), err)
	}

	// Success! Mark as active
	entry.pending = false
	entry.active = true
	entry.startedAt = time.Now()

	m.logger.Info("pending watch activated", "gvk", gvk.String())
	return nil
}

// isNotFoundError checks if the error indicates CRD is not available.
func isNotFoundError(err error) bool {
	// Check for "no matches for kind" error
	if meta.IsNoMatchError(err) {
		return true
	}
	// Check for discovery errors
	var discErr *discovery.ErrGroupDiscoveryFailed
	return errors.As(err, &discErr)
}
