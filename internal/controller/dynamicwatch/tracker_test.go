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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

const (
	addonUID1 = types.UID("addon-uid-1")
	addonUID2 = types.UID("addon-uid-2")
	addonUID3 = types.UID("addon-uid-3")
)

func TestNewTracker(t *testing.T) {
	tracker := NewTracker()
	require.NotNil(t, tracker)
	assert.Equal(t, 0, tracker.GetAddonCount())
	assert.Empty(t, tracker.GetAllGVKs())
}

func TestSetGVKs_NewAddon(t *testing.T) {
	tracker := NewTracker()
	gvks := NewGVKSet(gvkDeployment, gvkSecret)

	added, removed := tracker.SetGVKs(addonUID1, gvks)

	// All GVKs should be added, none removed
	assert.Equal(t, 2, added.Len())
	assert.True(t, added.Contains(gvkDeployment))
	assert.True(t, added.Contains(gvkSecret))
	assert.Equal(t, 0, removed.Len())

	// Verify stored GVKs
	stored := tracker.GetGVKs(addonUID1)
	assert.Equal(t, 2, stored.Len())
	assert.True(t, stored.Contains(gvkDeployment))
	assert.True(t, stored.Contains(gvkSecret))
}

func TestSetGVKs_UpdateAddon_AddOnly(t *testing.T) {
	tracker := NewTracker()

	// Initial set
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment))

	// Add more GVKs
	added, removed := tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret, gvkConfigMap))

	assert.Equal(t, 2, added.Len())
	assert.True(t, added.Contains(gvkSecret))
	assert.True(t, added.Contains(gvkConfigMap))
	assert.False(t, added.Contains(gvkDeployment))
	assert.Equal(t, 0, removed.Len())
}

func TestSetGVKs_UpdateAddon_RemoveOnly(t *testing.T) {
	tracker := NewTracker()

	// Initial set
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret, gvkConfigMap))

	// Remove some GVKs
	added, removed := tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment))

	assert.Equal(t, 0, added.Len())
	assert.Equal(t, 2, removed.Len())
	assert.True(t, removed.Contains(gvkSecret))
	assert.True(t, removed.Contains(gvkConfigMap))
}

func TestSetGVKs_UpdateAddon_Mixed(t *testing.T) {
	tracker := NewTracker()

	// Initial set: Deployment, Secret
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret))

	// Update: remove Secret, add ConfigMap, keep Deployment
	added, removed := tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkConfigMap))

	assert.Equal(t, 1, added.Len())
	assert.True(t, added.Contains(gvkConfigMap))
	assert.Equal(t, 1, removed.Len())
	assert.True(t, removed.Contains(gvkSecret))
}

func TestSetGVKs_ReplaceAll(t *testing.T) {
	tracker := NewTracker()

	// Initial set
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret))

	// Completely different set
	added, removed := tracker.SetGVKs(addonUID1, NewGVKSet(gvkConfigMap, gvkCert))

	assert.Equal(t, 2, added.Len())
	assert.True(t, added.Contains(gvkConfigMap))
	assert.True(t, added.Contains(gvkCert))
	assert.Equal(t, 2, removed.Len())
	assert.True(t, removed.Contains(gvkDeployment))
	assert.True(t, removed.Contains(gvkSecret))
}

func TestSetGVKs_EmptySet(t *testing.T) {
	tracker := NewTracker()

	// Set empty for new addon
	added, removed := tracker.SetGVKs(addonUID1, NewGVKSet())
	assert.Equal(t, 0, added.Len())
	assert.Equal(t, 0, removed.Len())

	// Set non-empty, then clear
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret))
	added, removed = tracker.SetGVKs(addonUID1, NewGVKSet())

	assert.Equal(t, 0, added.Len())
	assert.Equal(t, 2, removed.Len())
}

func TestGetGVKs_NotTracked(t *testing.T) {
	tracker := NewTracker()

	gvks := tracker.GetGVKs(addonUID1)

	assert.NotNil(t, gvks)
	assert.Equal(t, 0, gvks.Len())
}

func TestGetGVKs_ReturnsCopy(t *testing.T) {
	tracker := NewTracker()
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment))

	// Get GVKs and modify the result
	gvks := tracker.GetGVKs(addonUID1)
	gvks.Add(gvkSecret)

	// Original should not be affected
	original := tracker.GetGVKs(addonUID1)
	assert.Equal(t, 1, original.Len())
	assert.False(t, original.Contains(gvkSecret))
}

func TestRemoveAddon(t *testing.T) {
	tracker := NewTracker()
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret))
	tracker.SetGVKs(addonUID2, NewGVKSet(gvkConfigMap))

	assert.Equal(t, 2, tracker.GetAddonCount())

	// Remove addon1
	removed := tracker.RemoveAddon(addonUID1)

	assert.Equal(t, 2, removed.Len())
	assert.True(t, removed.Contains(gvkDeployment))
	assert.True(t, removed.Contains(gvkSecret))
	assert.Equal(t, 1, tracker.GetAddonCount())

	// Verify addon1 is no longer tracked
	assert.Equal(t, 0, tracker.GetGVKs(addonUID1).Len())
	// Verify addon2 is still tracked
	assert.Equal(t, 1, tracker.GetGVKs(addonUID2).Len())
}

func TestRemoveAddon_NotTracked(t *testing.T) {
	tracker := NewTracker()

	removed := tracker.RemoveAddon(addonUID1)

	assert.NotNil(t, removed)
	assert.Equal(t, 0, removed.Len())
}

func TestGetAllGVKs(t *testing.T) {
	tracker := NewTracker()

	// Empty tracker
	all := tracker.GetAllGVKs()
	assert.Equal(t, 0, all.Len())

	// Single addon
	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment, gvkSecret))
	all = tracker.GetAllGVKs()
	assert.Equal(t, 2, all.Len())

	// Multiple addons with overlapping GVKs
	tracker.SetGVKs(addonUID2, NewGVKSet(gvkSecret, gvkConfigMap))
	all = tracker.GetAllGVKs()
	assert.Equal(t, 3, all.Len()) // Deployment, Secret, ConfigMap (Secret not duplicated)
	assert.True(t, all.Contains(gvkDeployment))
	assert.True(t, all.Contains(gvkSecret))
	assert.True(t, all.Contains(gvkConfigMap))
}

func TestGetAddonCount(t *testing.T) {
	tracker := NewTracker()

	assert.Equal(t, 0, tracker.GetAddonCount())

	tracker.SetGVKs(addonUID1, NewGVKSet(gvkDeployment))
	assert.Equal(t, 1, tracker.GetAddonCount())

	tracker.SetGVKs(addonUID2, NewGVKSet(gvkSecret))
	assert.Equal(t, 2, tracker.GetAddonCount())

	tracker.RemoveAddon(addonUID1)
	assert.Equal(t, 1, tracker.GetAddonCount())
}

func TestTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewTracker()
	var wg sync.WaitGroup
	errChan := make(chan error, 300)

	// 100 goroutines setting GVKs
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			uid := types.UID(string(rune('a' + n%10)))
			tracker.SetGVKs(uid, NewGVKSet(gvkDeployment, gvkSecret))
		}(i)
	}

	// 100 goroutines getting GVKs
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			uid := types.UID(string(rune('a' + n%10)))
			_ = tracker.GetGVKs(uid)
		}(i)
	}

	// 100 goroutines getting all GVKs
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tracker.GetAllGVKs()
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent access error: %v", err)
	}
}

func TestSetGVKs_StoresCopy(t *testing.T) {
	tracker := NewTracker()
	gvks := NewGVKSet(gvkDeployment)

	tracker.SetGVKs(addonUID1, gvks)

	// Modify original set
	gvks.Add(gvkSecret)

	// Stored set should not be affected
	stored := tracker.GetGVKs(addonUID1)
	assert.Equal(t, 1, stored.Len())
	assert.False(t, stored.Contains(gvkSecret))
}

func BenchmarkSetGVKs(b *testing.B) {
	tracker := NewTracker()
	gvks := NewGVKSet(gvkDeployment, gvkSecret, gvkConfigMap)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.SetGVKs(addonUID1, gvks)
	}
}

func BenchmarkGetAllGVKs(b *testing.B) {
	tracker := NewTracker()
	// Add 100 addons with 5 GVKs each
	for i := 0; i < 100; i++ {
		uid := types.UID(string(rune('a' + i)))
		tracker.SetGVKs(uid, NewGVKSet(gvkDeployment, gvkSecret, gvkConfigMap, gvkCert))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.GetAllGVKs()
	}
}
