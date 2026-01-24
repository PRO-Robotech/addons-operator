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
	"fmt"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// mockController implements controller.Controller for testing.
type mockController struct {
	watchCallCount int
	watchError     error
	mu             sync.Mutex
}

func newMockController() *mockController {
	return &mockController{}
}

func (m *mockController) setWatchError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchError = err
}

func (m *mockController) getWatchCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.watchCallCount
}

func (m *mockController) Watch(src source.TypedSource[reconcile.Request]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchCallCount++
	return m.watchError
}

func (m *mockController) GetLogger() logr.Logger {
	return logr.Discard()
}

func (m *mockController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (m *mockController) Start(ctx context.Context) error {
	return nil
}

// testableManager extends manager for testing internal state.
type testableManager struct {
	*manager
}

func newTestableManager() *testableManager {
	m := &manager{
		watches: make(map[schema.GroupVersionKind]*watchEntry),
		logger:  logr.Discard(),
	}
	return &testableManager{manager: m}
}

func (t *testableManager) addWatch(gvk schema.GroupVersionKind, refCount int, active, pending bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.watches[gvk] = &watchEntry{
		gvk:      gvk,
		refCount: refCount,
		active:   active,
		pending:  pending,
	}
}

func (t *testableManager) getEntry(gvk schema.GroupVersionKind) *watchEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.watches[gvk]
}

func TestEnsureWatch_ExistingGVK_IncrementRefCount(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 1, true, false)

	// Call EnsureWatch on existing GVK
	err := tm.EnsureWatch(context.Background(), gvk)

	require.NoError(t, err)
	entry := tm.getEntry(gvk)
	require.NotNil(t, entry)
	assert.Equal(t, 2, entry.refCount)
}

func TestEnsureWatch_ExistingPendingGVK_IncrementRefCount(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}
	tm.addWatch(gvk, 1, false, true)

	err := tm.EnsureWatch(context.Background(), gvk)

	require.NoError(t, err)
	entry := tm.getEntry(gvk)
	require.NotNil(t, entry)
	assert.Equal(t, 2, entry.refCount)
	assert.True(t, entry.pending, "should remain pending")
}

func TestReleaseWatch_DecrementCount(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 3, true, false)

	tm.ReleaseWatch(gvk)

	entry := tm.getEntry(gvk)
	require.NotNil(t, entry)
	assert.Equal(t, 2, entry.refCount)
	assert.True(t, entry.active, "should still be active with refCount > 0")
}

func TestReleaseWatch_MarkInactiveAtZero(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 1, true, false)

	tm.ReleaseWatch(gvk)

	entry := tm.getEntry(gvk)
	require.NotNil(t, entry)
	assert.Equal(t, 0, entry.refCount)
	assert.False(t, entry.active, "should be marked inactive at refCount=0")
}

func TestReleaseWatch_UnknownGVK(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "unknown", Version: "v1", Kind: "Unknown"}

	// Should not panic
	tm.ReleaseWatch(gvk)

	entry := tm.getEntry(gvk)
	assert.Nil(t, entry)
}

func TestGetPendingGVKs(t *testing.T) {
	tm := newTestableManager()

	activeGVK := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	pendingGVK1 := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}
	pendingGVK2 := schema.GroupVersionKind{Group: "other.io", Version: "v1", Kind: "Other"}

	tm.addWatch(activeGVK, 1, true, false)
	tm.addWatch(pendingGVK1, 1, false, true)
	tm.addWatch(pendingGVK2, 1, false, true)

	pending := tm.GetPendingGVKs()

	assert.Len(t, pending, 2)
	assert.Contains(t, pending, pendingGVK1)
	assert.Contains(t, pending, pendingGVK2)
	assert.NotContains(t, pending, activeGVK)
}

func TestGetActiveGVKs(t *testing.T) {
	tm := newTestableManager()

	activeGVK1 := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	activeGVK2 := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	pendingGVK := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}
	inactiveGVK := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}

	tm.addWatch(activeGVK1, 2, true, false)
	tm.addWatch(activeGVK2, 1, true, false)
	tm.addWatch(pendingGVK, 1, false, true)
	tm.addWatch(inactiveGVK, 0, false, false)

	active := tm.GetActiveGVKs()

	assert.Len(t, active, 2)
	assert.Contains(t, active, activeGVK1)
	assert.Contains(t, active, activeGVK2)
	assert.NotContains(t, active, pendingGVK)
	assert.NotContains(t, active, inactiveGVK)
}

func TestGetWatchCount(t *testing.T) {
	tm := newTestableManager()

	assert.Equal(t, 0, tm.GetWatchCount())

	tm.addWatch(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, 1, true, false)
	assert.Equal(t, 1, tm.GetWatchCount())

	tm.addWatch(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, 1, true, false)
	assert.Equal(t, 2, tm.GetWatchCount())
}

func TestConcurrentAccess(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 100, true, false)

	var wg sync.WaitGroup
	errChan := make(chan error, 200)

	// 100 goroutines incrementing
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tm.EnsureWatch(context.Background(), gvk); err != nil {
				errChan <- err
			}
		}()
	}

	// 100 goroutines decrementing
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tm.ReleaseWatch(gvk)
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent access error: %v", err)
	}

	// RefCount should be back to 100 (100 + 100 - 100)
	entry := tm.getEntry(gvk)
	require.NotNil(t, entry)
	assert.Equal(t, 100, entry.refCount)
}

func TestIsNotFoundError_NoMatchError(t *testing.T) {
	err := &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "custom.io", Kind: "MyResource"},
		SearchedVersions: []string{"v1"},
	}

	assert.True(t, isNotFoundError(err))
}

func TestIsNotFoundError_RegularError(t *testing.T) {
	err := fmt.Errorf("some other error")

	assert.False(t, isNotFoundError(err))
}

func TestIsNotFoundError_Nil(t *testing.T) {
	assert.False(t, isNotFoundError(nil))
}

func TestNewWatchManager(t *testing.T) {
	mockCtrl := newMockController()
	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())

	require.NotNil(t, wm)
	assert.Equal(t, 0, wm.GetWatchCount())
	assert.Empty(t, wm.GetActiveGVKs())
	assert.Empty(t, wm.GetPendingGVKs())
}

func TestReleaseWatch_MultipleReleases(t *testing.T) {
	tm := newTestableManager()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 2, true, false)

	// First release
	tm.ReleaseWatch(gvk)
	entry := tm.getEntry(gvk)
	assert.Equal(t, 1, entry.refCount)
	assert.True(t, entry.active)

	// Second release
	tm.ReleaseWatch(gvk)
	entry = tm.getEntry(gvk)
	assert.Equal(t, 0, entry.refCount)
	assert.False(t, entry.active)

	// Third release (below zero)
	tm.ReleaseWatch(gvk)
	entry = tm.getEntry(gvk)
	assert.Equal(t, -1, entry.refCount)
	assert.False(t, entry.active)
}

func BenchmarkEnsureWatch_CacheHit(b *testing.B) {
	tm := newTestableManager()
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 1, true, false)

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = tm.EnsureWatch(ctx, gvk)
	}
}

func BenchmarkGetActiveGVKs(b *testing.B) {
	tm := newTestableManager()

	// Add 100 watches
	for i := 0; i < 100; i++ {
		gvk := schema.GroupVersionKind{
			Group:   fmt.Sprintf("group%d", i),
			Version: "v1",
			Kind:    fmt.Sprintf("Kind%d", i),
		}
		tm.addWatch(gvk, 1, i%2 == 0, i%2 != 0)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = tm.GetActiveGVKs()
	}
}

func TestEnsureWatch_NewGVK_Success(t *testing.T) {
	mockCtrl := newMockController()
	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())
	m := wm.(*manager)

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	err := wm.EnsureWatch(context.Background(), gvk)

	require.NoError(t, err)
	assert.Equal(t, 1, mockCtrl.getWatchCallCount())
	assert.Equal(t, 1, wm.GetWatchCount())

	// Check entry state
	m.mu.RLock()
	entry := m.watches[gvk]
	m.mu.RUnlock()

	require.NotNil(t, entry)
	assert.Equal(t, 1, entry.refCount)
	assert.True(t, entry.active)
	assert.False(t, entry.pending)
}

func TestEnsureWatch_NewGVK_CRDNotAvailable(t *testing.T) {
	mockCtrl := newMockController()
	mockCtrl.setWatchError(&meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "custom.io", Kind: "MyResource"},
		SearchedVersions: []string{"v1"},
	})

	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())
	m := wm.(*manager)

	gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}

	err := wm.EnsureWatch(context.Background(), gvk)

	// Should not return error (graceful degradation)
	require.NoError(t, err)
	assert.Equal(t, 1, wm.GetWatchCount())

	// Check entry is marked as pending
	m.mu.RLock()
	entry := m.watches[gvk]
	m.mu.RUnlock()

	require.NotNil(t, entry)
	assert.Equal(t, 1, entry.refCount)
	assert.False(t, entry.active)
	assert.True(t, entry.pending)

	// Should be in pending list
	pending := wm.GetPendingGVKs()
	assert.Contains(t, pending, gvk)
}

func TestEnsureWatch_NewGVK_OtherError(t *testing.T) {
	mockCtrl := newMockController()
	mockCtrl.setWatchError(fmt.Errorf("some unexpected error"))

	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	err := wm.EnsureWatch(context.Background(), gvk)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "start watch for")
	assert.Equal(t, 0, wm.GetWatchCount())
}

func TestEnsureWatch_SecondCallDoesNotCreateNewWatch(t *testing.T) {
	mockCtrl := newMockController()
	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	// First call creates watch
	err := wm.EnsureWatch(context.Background(), gvk)
	require.NoError(t, err)
	assert.Equal(t, 1, mockCtrl.getWatchCallCount())

	// Second call should not create new watch, just increment refCount
	err = wm.EnsureWatch(context.Background(), gvk)
	require.NoError(t, err)
	assert.Equal(t, 1, mockCtrl.getWatchCallCount(), "should not call Watch again")

	m := wm.(*manager)
	m.mu.RLock()
	entry := m.watches[gvk]
	m.mu.RUnlock()
	assert.Equal(t, 2, entry.refCount)
}

func TestRetryPendingWatch_CRDNowAvailable(t *testing.T) {
	mockCtrl := newMockController()
	// First call will fail with NoKindMatchError, subsequent calls will succeed
	mockCtrl.setWatchError(&meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "custom.io", Kind: "MyResource"},
		SearchedVersions: []string{"v1"},
	})

	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())
	m := wm.(*manager)

	gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}

	// Initial EnsureWatch marks as pending
	err := wm.EnsureWatch(context.Background(), gvk)
	require.NoError(t, err)
	assert.True(t, m.watches[gvk].pending)

	// Now CRD is available
	mockCtrl.setWatchError(nil)

	// Retry should succeed
	err = wm.RetryPendingWatch(context.Background(), gvk)
	require.NoError(t, err)

	// Check entry is now active
	m.mu.RLock()
	entry := m.watches[gvk]
	m.mu.RUnlock()

	assert.False(t, entry.pending, "should no longer be pending")
	assert.True(t, entry.active, "should be active")
	assert.Equal(t, 1, entry.refCount, "refCount should be preserved")
}

func TestRetryPendingWatch_CRDStillUnavailable(t *testing.T) {
	mockCtrl := newMockController()
	mockCtrl.setWatchError(&meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "custom.io", Kind: "MyResource"},
		SearchedVersions: []string{"v1"},
	})

	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())
	m := wm.(*manager)

	gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}

	// Initial EnsureWatch marks as pending
	err := wm.EnsureWatch(context.Background(), gvk)
	require.NoError(t, err)

	// Retry while CRD still unavailable
	err = wm.RetryPendingWatch(context.Background(), gvk)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CRD still not available")

	// Check entry is still pending
	m.mu.RLock()
	entry := m.watches[gvk]
	m.mu.RUnlock()

	assert.True(t, entry.pending, "should still be pending")
	assert.False(t, entry.active, "should not be active")
}

func TestRetryPendingWatch_NotPending(t *testing.T) {
	tm := newTestableManager()

	// Add an active (not pending) watch
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tm.addWatch(gvk, 1, true, false)

	// RetryPendingWatch on non-pending should return nil (nothing to do)
	err := tm.RetryPendingWatch(context.Background(), gvk)
	require.NoError(t, err)
}

func TestRetryPendingWatch_UnknownGVK(t *testing.T) {
	tm := newTestableManager()

	// Try to retry unknown GVK
	gvk := schema.GroupVersionKind{Group: "unknown", Version: "v1", Kind: "Unknown"}

	err := tm.RetryPendingWatch(context.Background(), gvk)
	require.NoError(t, err) // Should return nil for unknown GVK
}

func TestRetryPendingWatch_OtherError(t *testing.T) {
	mockCtrl := newMockController()
	// First call: NoKindMatchError
	mockCtrl.setWatchError(&meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "custom.io", Kind: "MyResource"},
		SearchedVersions: []string{"v1"},
	})

	mapFunc := func(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
		return nil
	}

	wm := NewWatchManager(nil, mockCtrl, mapFunc, logr.Discard())

	gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "MyResource"}

	// Initial EnsureWatch marks as pending
	err := wm.EnsureWatch(context.Background(), gvk)
	require.NoError(t, err)

	// Now a different error occurs
	mockCtrl.setWatchError(fmt.Errorf("unexpected error"))

	err = wm.RetryPendingWatch(context.Background(), gvk)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start watch for")
}
