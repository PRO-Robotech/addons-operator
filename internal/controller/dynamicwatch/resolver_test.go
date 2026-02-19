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
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func TestResolve_CoreTypes(t *testing.T) {
	resolver := NewResolver()

	tests := []struct {
		name       string
		apiVersion string
		kind       string
		wantGroup  string
		wantVer    string
	}{
		{
			name:       "Secret",
			apiVersion: "v1",
			kind:       "Secret",
			wantGroup:  "",
			wantVer:    "v1",
		},
		{
			name:       "ConfigMap",
			apiVersion: "v1",
			kind:       "ConfigMap",
			wantGroup:  "",
			wantVer:    "v1",
		},
		{
			name:       "Service",
			apiVersion: "v1",
			kind:       "Service",
			wantGroup:  "",
			wantVer:    "v1",
		},
		{
			name:       "Pod",
			apiVersion: "v1",
			kind:       "Pod",
			wantGroup:  "",
			wantVer:    "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := addonsv1alpha1.SourceRef{
				APIVersion: tt.apiVersion,
				Kind:       tt.kind,
				Name:       "test",
			}

			gvk, err := resolver.Resolve(ref)

			require.NoError(t, err)
			assert.Equal(t, tt.wantGroup, gvk.Group)
			assert.Equal(t, tt.wantVer, gvk.Version)
			assert.Equal(t, tt.kind, gvk.Kind)
		})
	}
}

func TestResolve_StandardTypes(t *testing.T) {
	resolver := NewResolver()

	tests := []struct {
		name       string
		apiVersion string
		kind       string
		wantGroup  string
		wantVer    string
	}{
		{
			name:       "Deployment",
			apiVersion: "apps/v1",
			kind:       "Deployment",
			wantGroup:  "apps",
			wantVer:    "v1",
		},
		{
			name:       "StatefulSet",
			apiVersion: "apps/v1",
			kind:       "StatefulSet",
			wantGroup:  "apps",
			wantVer:    "v1",
		},
		{
			name:       "Job",
			apiVersion: "batch/v1",
			kind:       "Job",
			wantGroup:  "batch",
			wantVer:    "v1",
		},
		{
			name:       "CronJob",
			apiVersion: "batch/v1",
			kind:       "CronJob",
			wantGroup:  "batch",
			wantVer:    "v1",
		},
		{
			name:       "Ingress",
			apiVersion: "networking.k8s.io/v1",
			kind:       "Ingress",
			wantGroup:  "networking.k8s.io",
			wantVer:    "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := addonsv1alpha1.SourceRef{
				APIVersion: tt.apiVersion,
				Kind:       tt.kind,
				Name:       "test",
			}

			gvk, err := resolver.Resolve(ref)

			require.NoError(t, err)
			assert.Equal(t, tt.wantGroup, gvk.Group)
			assert.Equal(t, tt.wantVer, gvk.Version)
			assert.Equal(t, tt.kind, gvk.Kind)
		})
	}
}

func TestResolve_CRDTypes(t *testing.T) {
	resolver := NewResolver()

	tests := []struct {
		name       string
		apiVersion string
		kind       string
		wantGroup  string
		wantVer    string
	}{
		{
			name:       "Certificate",
			apiVersion: "cert-manager.io/v1",
			kind:       "Certificate",
			wantGroup:  "cert-manager.io",
			wantVer:    "v1",
		},
		{
			name:       "VaultSecret",
			apiVersion: "vault.hashicorp.com/v1",
			kind:       "VaultSecret",
			wantGroup:  "vault.hashicorp.com",
			wantVer:    "v1",
		},
		{
			name:       "Addon",
			apiVersion: "addons.in-cloud.io/v1alpha1",
			kind:       "Addon",
			wantGroup:  "addons.in-cloud.io",
			wantVer:    "v1alpha1",
		},
		{
			name:       "CustomResource with beta version",
			apiVersion: "custom.example.io/v1beta1",
			kind:       "MyResource",
			wantGroup:  "custom.example.io",
			wantVer:    "v1beta1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := addonsv1alpha1.SourceRef{
				APIVersion: tt.apiVersion,
				Kind:       tt.kind,
				Name:       "test",
			}

			gvk, err := resolver.Resolve(ref)

			require.NoError(t, err)
			assert.Equal(t, tt.wantGroup, gvk.Group)
			assert.Equal(t, tt.wantVer, gvk.Version)
			assert.Equal(t, tt.kind, gvk.Kind)
		})
	}
}

func TestResolve_MalformedAPIVersion(t *testing.T) {
	resolver := NewResolver()

	tests := []struct {
		name       string
		apiVersion string
		kind       string
		wantErr    bool
	}{
		{
			name:       "invalid format with too many slashes",
			apiVersion: "apps/v1/extra",
			kind:       "Deployment",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := addonsv1alpha1.SourceRef{
				APIVersion: tt.apiVersion,
				Kind:       tt.kind,
				Name:       "test",
			}

			_, err := resolver.Resolve(ref)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResolve_EmptyAPIVersion(t *testing.T) {
	resolver := NewResolver()

	// Empty apiVersion is technically valid per schema.ParseGroupVersion
	// It produces an empty GroupVersion (Group: "", Version: "")
	ref := addonsv1alpha1.SourceRef{
		APIVersion: "",
		Kind:       "Secret",
		Name:       "test",
	}

	gvk, err := resolver.Resolve(ref)

	// schema.ParseGroupVersion accepts empty string
	require.NoError(t, err)
	assert.Equal(t, "", gvk.Group)
	assert.Equal(t, "", gvk.Version)
	assert.Equal(t, "Secret", gvk.Kind)
}

func TestResolve_Caching(t *testing.T) {
	r := NewResolver().(*resolver)

	ref := addonsv1alpha1.SourceRef{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "test",
	}

	// First call - should parse and cache
	gvk1, err := r.Resolve(ref)
	require.NoError(t, err)

	// Verify cache is populated
	r.mu.RLock()
	cacheLen := len(r.cache)
	r.mu.RUnlock()
	assert.Equal(t, 1, cacheLen)

	// Second call - should use cache
	gvk2, err := r.Resolve(ref)
	require.NoError(t, err)

	// Results should be identical
	assert.Equal(t, gvk1, gvk2)

	// Cache should still have 1 entry (not 2)
	r.mu.RLock()
	cacheLen = len(r.cache)
	r.mu.RUnlock()
	assert.Equal(t, 1, cacheLen)

	// Different ref should add to cache
	ref2 := addonsv1alpha1.SourceRef{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       "test",
	}
	_, err = r.Resolve(ref2)
	require.NoError(t, err)

	r.mu.RLock()
	cacheLen = len(r.cache)
	r.mu.RUnlock()
	assert.Equal(t, 2, cacheLen)
}

func TestResolve_ConcurrentAccess(t *testing.T) {
	resolver := NewResolver()

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// Spawn 100 goroutines resolving different refs
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			ref := addonsv1alpha1.SourceRef{
				APIVersion: fmt.Sprintf("group%d/v1", n%10),
				Kind:       fmt.Sprintf("Kind%d", n%5),
				Name:       "test",
			}

			gvk, err := resolver.Resolve(ref)
			if err != nil {
				errChan <- err

				return
			}

			// Verify result is correct
			expectedGroup := fmt.Sprintf("group%d", n%10)
			if gvk.Group != expectedGroup {
				errChan <- fmt.Errorf("expected group %s, got %s", expectedGroup, gvk.Group)
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		t.Errorf("concurrent access error: %v", err)
	}
}

func TestResolve_SameKindDifferentAPIVersion(t *testing.T) {
	resolver := NewResolver()

	// Same kind but different API versions should produce different GVKs
	ref1 := addonsv1alpha1.SourceRef{
		APIVersion: "networking.k8s.io/v1",
		Kind:       "Ingress",
		Name:       "test",
	}
	ref2 := addonsv1alpha1.SourceRef{
		APIVersion: "networking.k8s.io/v1beta1",
		Kind:       "Ingress",
		Name:       "test",
	}

	gvk1, err := resolver.Resolve(ref1)
	require.NoError(t, err)

	gvk2, err := resolver.Resolve(ref2)
	require.NoError(t, err)

	assert.NotEqual(t, gvk1, gvk2)
	assert.Equal(t, "v1", gvk1.Version)
	assert.Equal(t, "v1beta1", gvk2.Version)
}

func TestResolve_NamespaceIsIgnored(t *testing.T) {
	resolver := NewResolver()

	// Namespace should not affect GVK resolution
	ref1 := addonsv1alpha1.SourceRef{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       "test",
		Namespace:  "ns1",
	}
	ref2 := addonsv1alpha1.SourceRef{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       "test",
		Namespace:  "ns2",
	}

	gvk1, err := resolver.Resolve(ref1)
	require.NoError(t, err)

	gvk2, err := resolver.Resolve(ref2)
	require.NoError(t, err)

	assert.Equal(t, gvk1, gvk2)
}

func BenchmarkResolve_CacheHit(b *testing.B) {
	resolver := NewResolver()

	ref := addonsv1alpha1.SourceRef{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "test",
	}

	// Warm up cache
	_, _ = resolver.Resolve(ref)

	for b.Loop() {
		_, _ = resolver.Resolve(ref)
	}
}

func BenchmarkResolve_CacheMiss(b *testing.B) {
	for b.Loop() {
		resolver := NewResolver()
		ref := addonsv1alpha1.SourceRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "test",
		}
		_, _ = resolver.Resolve(ref)
	}
}

func TestNewResolver(t *testing.T) {
	resolver := NewResolver()
	assert.NotNil(t, resolver)

	// Should be able to resolve immediately
	ref := addonsv1alpha1.SourceRef{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       "test",
	}
	gvk, err := resolver.Resolve(ref)
	require.NoError(t, err)
	assert.Equal(t, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, gvk)
}
