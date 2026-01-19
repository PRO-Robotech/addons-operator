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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GVKSet is a set of GroupVersionKinds.
type GVKSet map[schema.GroupVersionKind]struct{}

// NewGVKSet creates a GVKSet from a slice of GVKs.
func NewGVKSet(gvks ...schema.GroupVersionKind) GVKSet {
	set := make(GVKSet, len(gvks))
	for _, gvk := range gvks {
		set[gvk] = struct{}{}
	}
	return set
}

// Add adds a GVK to the set.
func (s GVKSet) Add(gvk schema.GroupVersionKind) {
	s[gvk] = struct{}{}
}

// Contains returns true if the set contains the GVK.
func (s GVKSet) Contains(gvk schema.GroupVersionKind) bool {
	_, exists := s[gvk]
	return exists
}

// Len returns the number of GVKs in the set.
func (s GVKSet) Len() int {
	return len(s)
}

// Slice returns the GVKSet as a slice.
func (s GVKSet) Slice() []schema.GroupVersionKind {
	result := make([]schema.GroupVersionKind, 0, len(s))
	for gvk := range s {
		result = append(result, gvk)
	}
	return result
}

// Difference returns GVKs in s but not in other.
func (s GVKSet) Difference(other GVKSet) GVKSet {
	result := make(GVKSet)
	for gvk := range s {
		if !other.Contains(gvk) {
			result[gvk] = struct{}{}
		}
	}
	return result
}

// Union returns GVKs that are in either set.
func (s GVKSet) Union(other GVKSet) GVKSet {
	result := make(GVKSet, len(s)+len(other))
	for gvk := range s {
		result[gvk] = struct{}{}
	}
	for gvk := range other {
		result[gvk] = struct{}{}
	}
	return result
}

// Copy returns a copy of the set.
func (s GVKSet) Copy() GVKSet {
	result := make(GVKSet, len(s))
	for gvk := range s {
		result[gvk] = struct{}{}
	}
	return result
}
