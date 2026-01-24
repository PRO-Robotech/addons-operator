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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	gvkDeployment = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	gvkSecret     = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	gvkConfigMap  = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	gvkCert       = schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"}
)

func TestNewGVKSet(t *testing.T) {
	tests := []struct {
		name     string
		gvks     []schema.GroupVersionKind
		wantLen  int
		wantGVKs []schema.GroupVersionKind
	}{
		{
			name:     "empty",
			gvks:     nil,
			wantLen:  0,
			wantGVKs: nil,
		},
		{
			name:     "single",
			gvks:     []schema.GroupVersionKind{gvkDeployment},
			wantLen:  1,
			wantGVKs: []schema.GroupVersionKind{gvkDeployment},
		},
		{
			name:     "multiple",
			gvks:     []schema.GroupVersionKind{gvkDeployment, gvkSecret, gvkConfigMap},
			wantLen:  3,
			wantGVKs: []schema.GroupVersionKind{gvkDeployment, gvkSecret, gvkConfigMap},
		},
		{
			name:     "duplicates",
			gvks:     []schema.GroupVersionKind{gvkDeployment, gvkDeployment, gvkSecret},
			wantLen:  2,
			wantGVKs: []schema.GroupVersionKind{gvkDeployment, gvkSecret},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := NewGVKSet(tt.gvks...)
			assert.Equal(t, tt.wantLen, set.Len())
			for _, gvk := range tt.wantGVKs {
				assert.True(t, set.Contains(gvk), "should contain %v", gvk)
			}
		})
	}
}

func TestGVKSet_Add(t *testing.T) {
	set := NewGVKSet()
	assert.Equal(t, 0, set.Len())

	set.Add(gvkDeployment)
	assert.Equal(t, 1, set.Len())
	assert.True(t, set.Contains(gvkDeployment))

	// Add same GVK again
	set.Add(gvkDeployment)
	assert.Equal(t, 1, set.Len())

	// Add different GVK
	set.Add(gvkSecret)
	assert.Equal(t, 2, set.Len())
	assert.True(t, set.Contains(gvkSecret))
}

func TestGVKSet_Contains(t *testing.T) {
	set := NewGVKSet(gvkDeployment, gvkSecret)

	assert.True(t, set.Contains(gvkDeployment))
	assert.True(t, set.Contains(gvkSecret))
	assert.False(t, set.Contains(gvkConfigMap))
	assert.False(t, set.Contains(gvkCert))
}

func TestGVKSet_Slice(t *testing.T) {
	tests := []struct {
		name    string
		gvks    []schema.GroupVersionKind
		wantLen int
	}{
		{
			name:    "empty",
			gvks:    nil,
			wantLen: 0,
		},
		{
			name:    "single",
			gvks:    []schema.GroupVersionKind{gvkDeployment},
			wantLen: 1,
		},
		{
			name:    "multiple",
			gvks:    []schema.GroupVersionKind{gvkDeployment, gvkSecret, gvkConfigMap},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := NewGVKSet(tt.gvks...)
			slice := set.Slice()
			assert.Len(t, slice, tt.wantLen)

			// Verify all original GVKs are in slice
			sliceSet := NewGVKSet(slice...)
			for _, gvk := range tt.gvks {
				assert.True(t, sliceSet.Contains(gvk))
			}
		})
	}
}

func TestGVKSet_Difference(t *testing.T) {
	tests := []struct {
		name     string
		set1     GVKSet
		set2     GVKSet
		wantGVKs []schema.GroupVersionKind
	}{
		{
			name:     "both empty",
			set1:     NewGVKSet(),
			set2:     NewGVKSet(),
			wantGVKs: nil,
		},
		{
			name:     "first empty",
			set1:     NewGVKSet(),
			set2:     NewGVKSet(gvkDeployment),
			wantGVKs: nil,
		},
		{
			name:     "second empty",
			set1:     NewGVKSet(gvkDeployment, gvkSecret),
			set2:     NewGVKSet(),
			wantGVKs: []schema.GroupVersionKind{gvkDeployment, gvkSecret},
		},
		{
			name:     "some overlap",
			set1:     NewGVKSet(gvkDeployment, gvkSecret, gvkConfigMap),
			set2:     NewGVKSet(gvkSecret, gvkCert),
			wantGVKs: []schema.GroupVersionKind{gvkDeployment, gvkConfigMap},
		},
		{
			name:     "complete overlap",
			set1:     NewGVKSet(gvkDeployment, gvkSecret),
			set2:     NewGVKSet(gvkDeployment, gvkSecret, gvkConfigMap),
			wantGVKs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := tt.set1.Difference(tt.set2)
			assert.Equal(t, len(tt.wantGVKs), diff.Len())
			for _, gvk := range tt.wantGVKs {
				assert.True(t, diff.Contains(gvk), "should contain %v", gvk)
			}
		})
	}
}

func TestGVKSet_Union(t *testing.T) {
	tests := []struct {
		name     string
		set1     GVKSet
		set2     GVKSet
		wantLen  int
		wantGVKs []schema.GroupVersionKind
	}{
		{
			name:     "both empty",
			set1:     NewGVKSet(),
			set2:     NewGVKSet(),
			wantLen:  0,
			wantGVKs: nil,
		},
		{
			name:     "first empty",
			set1:     NewGVKSet(),
			set2:     NewGVKSet(gvkDeployment),
			wantLen:  1,
			wantGVKs: []schema.GroupVersionKind{gvkDeployment},
		},
		{
			name:     "disjoint",
			set1:     NewGVKSet(gvkDeployment, gvkSecret),
			set2:     NewGVKSet(gvkConfigMap, gvkCert),
			wantLen:  4,
			wantGVKs: []schema.GroupVersionKind{gvkDeployment, gvkSecret, gvkConfigMap, gvkCert},
		},
		{
			name:     "overlap",
			set1:     NewGVKSet(gvkDeployment, gvkSecret),
			set2:     NewGVKSet(gvkSecret, gvkConfigMap),
			wantLen:  3,
			wantGVKs: []schema.GroupVersionKind{gvkDeployment, gvkSecret, gvkConfigMap},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			union := tt.set1.Union(tt.set2)
			assert.Equal(t, tt.wantLen, union.Len())
			for _, gvk := range tt.wantGVKs {
				assert.True(t, union.Contains(gvk), "should contain %v", gvk)
			}
		})
	}
}

func TestGVKSet_Copy(t *testing.T) {
	original := NewGVKSet(gvkDeployment, gvkSecret)
	copied := original.Copy()

	// Should have same content
	assert.Equal(t, original.Len(), copied.Len())
	assert.True(t, copied.Contains(gvkDeployment))
	assert.True(t, copied.Contains(gvkSecret))

	// Modifications to copy should not affect original
	copied.Add(gvkConfigMap)
	assert.Equal(t, 3, copied.Len())
	assert.Equal(t, 2, original.Len())
	assert.False(t, original.Contains(gvkConfigMap))
}
