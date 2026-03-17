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
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// Tracker tracks which GVKs each Addon is watching.
// It maintains a mapping from Addon UID to its current GVK set.
type Tracker interface {
	// SetGVKs updates the GVK set for an Addon.
	// Returns (added, removed) GVKs based on diff with previous state.
	SetGVKs(addonUID types.UID, gvks GVKSet) (added, removed GVKSet)

	// GetGVKs returns current GVKs for an Addon.
	// Returns empty set if Addon is not tracked.
	GetGVKs(addonUID types.UID) GVKSet

	// RemoveAddon removes tracking for an Addon.
	// Returns GVKs that were tracked (for releasing watches).
	RemoveAddon(addonUID types.UID) GVKSet

	// GetAllGVKs returns union of all GVKs across all Addons.
	GetAllGVKs() GVKSet

	// GetAddonCount returns number of tracked Addons.
	GetAddonCount() int
}

// tracker implements Tracker with thread-safe operations.
type tracker struct {
	addonGVKs map[types.UID]GVKSet
	mu        sync.RWMutex
}

// NewTracker creates a new tracker.
func NewTracker() Tracker {
	return &tracker{
		addonGVKs: make(map[types.UID]GVKSet),
	}
}

// SetGVKs updates the GVK set for an Addon and returns the diff.
func (t *tracker) SetGVKs(addonUID types.UID, newGVKs GVKSet) (added, removed GVKSet) {
	t.mu.Lock()
	defer t.mu.Unlock()

	oldGVKs := t.addonGVKs[addonUID]
	if oldGVKs == nil {
		oldGVKs = make(GVKSet)
	}

	// Calculate diff
	added = newGVKs.Difference(oldGVKs)
	removed = oldGVKs.Difference(newGVKs)

	// Store copy of new GVKs
	t.addonGVKs[addonUID] = newGVKs.Copy()

	return added, removed
}

// GetGVKs returns current GVKs for an Addon.
func (t *tracker) GetGVKs(addonUID types.UID) GVKSet {
	t.mu.RLock()
	defer t.mu.RUnlock()

	gvks := t.addonGVKs[addonUID]
	if gvks == nil {
		return make(GVKSet)
	}

	return gvks.Copy()
}

// RemoveAddon removes tracking for an Addon.
func (t *tracker) RemoveAddon(addonUID types.UID) GVKSet {
	t.mu.Lock()
	defer t.mu.Unlock()

	gvks := t.addonGVKs[addonUID]
	delete(t.addonGVKs, addonUID)

	if gvks == nil {
		return make(GVKSet)
	}

	return gvks
}

// GetAllGVKs returns union of all GVKs across all Addons.
func (t *tracker) GetAllGVKs() GVKSet {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(GVKSet)
	for _, gvks := range t.addonGVKs {
		for gvk := range gvks {
			result[gvk] = struct{}{}
		}
	}

	return result
}

// GetAddonCount returns number of tracked Addons.
func (t *tracker) GetAddonCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.addonGVKs)
}
