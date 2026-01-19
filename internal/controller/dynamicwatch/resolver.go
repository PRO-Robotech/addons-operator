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

	"k8s.io/apimachinery/pkg/runtime/schema"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// Resolver converts SourceRef to GroupVersionKind.
// The result is cached for performance.
type Resolver interface {
	// Resolve converts SourceRef to GVK.
	// Returns error if apiVersion is malformed.
	Resolve(ref addonsv1alpha1.SourceRef) (schema.GroupVersionKind, error)
}

// resolver implements Resolver with thread-safe caching.
type resolver struct {
	cache map[string]schema.GroupVersionKind
	mu    sync.RWMutex
}

// NewResolver creates a new GVK resolver with an empty cache.
func NewResolver() Resolver {
	return &resolver{
		cache: make(map[string]schema.GroupVersionKind),
	}
}

// Resolve converts SourceRef to GroupVersionKind.
// Results are cached using apiVersion/kind as key.
//
// Core types (apiVersion: v1):
//   - v1/Secret     → {Group: "", Version: "v1", Kind: "Secret"}
//   - v1/ConfigMap  → {Group: "", Version: "v1", Kind: "ConfigMap"}
//
// Standard types (apiVersion: group/version):
//   - apps/v1/Deployment → {Group: "apps", Version: "v1", Kind: "Deployment"}
//
// CRD types:
//   - cert-manager.io/v1/Certificate → {Group: "cert-manager.io", Version: "v1", Kind: "Certificate"}
func (r *resolver) Resolve(ref addonsv1alpha1.SourceRef) (schema.GroupVersionKind, error) {
	key := ref.APIVersion + "/" + ref.Kind

	// Fast path: check cache with read lock
	r.mu.RLock()
	if gvk, ok := r.cache[key]; ok {
		r.mu.RUnlock()
		return gvk, nil
	}
	r.mu.RUnlock()

	// Parse apiVersion into Group and Version
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("parse apiVersion %q: %w", ref.APIVersion, err)
	}

	gvk := gv.WithKind(ref.Kind)

	// Cache result with write lock
	r.mu.Lock()
	r.cache[key] = gvk
	r.mu.Unlock()

	return gvk, nil
}
