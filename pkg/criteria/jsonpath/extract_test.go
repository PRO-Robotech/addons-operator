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

package jsonpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract(t *testing.T) {
	// Sample Kubernetes-like object
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "test-pod",
			"namespace": "default",
			"labels": map[string]any{
				"app":                       "myapp",
				"app.kubernetes.io/name":    "myapp",
				"app.kubernetes.io/part-of": "mysuite",
			},
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "main",
					"image": "nginx:latest",
				},
				map[string]any{
					"name":  "sidecar",
					"image": "envoy:v1.0",
				},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
				map[string]any{"type": "Initialized", "status": "True"},
				map[string]any{"type": "ContainersReady", "status": "True"},
			},
		},
	}

	tests := []struct {
		name     string
		segments []PathSegment
		expected any
		found    bool
	}{
		{
			name:     "empty segments returns root",
			segments: nil,
			expected: obj,
			found:    true,
		},
		{
			name: "simple field",
			segments: []PathSegment{
				FieldSegment{Field: "kind"},
			},
			expected: "Pod",
			found:    true,
		},
		{
			name: "nested fields",
			segments: []PathSegment{
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "name"},
			},
			expected: "test-pod",
			found:    true,
		},
		{
			name: "deep nesting",
			segments: []PathSegment{
				FieldSegment{Field: "status"},
				FieldSegment{Field: "phase"},
			},
			expected: "Running",
			found:    true,
		},
		{
			name: "array index",
			segments: []PathSegment{
				FieldSegment{Field: "spec"},
				FieldSegment{Field: "containers"},
				IndexSegment{Index: 0},
				FieldSegment{Field: "name"},
			},
			expected: "main",
			found:    true,
		},
		{
			name: "bracket notation",
			segments: []PathSegment{
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "labels"},
				BracketSegment{Key: "app.kubernetes.io/name"},
			},
			expected: "myapp",
			found:    true,
		},
		{
			name: "filter expression",
			segments: []PathSegment{
				FieldSegment{Field: "status"},
				FieldSegment{Field: "conditions"},
				FilterSegment{Field: "type", Operator: "==", Value: "Ready"},
				FieldSegment{Field: "status"},
			},
			expected: "True",
			found:    true,
		},
		{
			name: "field not found",
			segments: []PathSegment{
				FieldSegment{Field: "nonexistent"},
			},
			expected: nil,
			found:    false,
		},
		{
			name: "nested field not found",
			segments: []PathSegment{
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "nonexistent"},
			},
			expected: nil,
			found:    false,
		},
		{
			name: "array index out of bounds",
			segments: []PathSegment{
				FieldSegment{Field: "spec"},
				FieldSegment{Field: "containers"},
				IndexSegment{Index: 99},
			},
			expected: nil,
			found:    false,
		},
		{
			name: "filter no match",
			segments: []PathSegment{
				FieldSegment{Field: "status"},
				FieldSegment{Field: "conditions"},
				FilterSegment{Field: "type", Operator: "==", Value: "NonExistent"},
			},
			expected: nil,
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found, err := Extract(obj, tt.segments)
			require.NoError(t, err)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractString(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name": "test",
			"labels": map[string]any{
				"app.kubernetes.io/name": "myapp",
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}

	tests := []struct {
		name     string
		path     string
		expected any
		found    bool
		wantErr  bool
	}{
		{
			name:     "simple path",
			path:     "/metadata/name",
			expected: "test",
			found:    true,
		},
		{
			name:     "bracket path",
			path:     `/metadata/labels["app.kubernetes.io/name"]`,
			expected: "myapp",
			found:    true,
		},
		{
			name:     "filter path",
			path:     `/status/conditions[?(@.type=='Ready')]/status`,
			expected: "True",
			found:    true,
		},
		{
			name:     "not found",
			path:     "/nonexistent",
			expected: nil,
			found:    false,
		},
		{
			name:    "invalid path",
			path:    "no-leading-slash",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found, err := ExtractString(obj, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtract_EdgeCases(t *testing.T) {
	t.Run("nil object", func(t *testing.T) {
		segments := []PathSegment{FieldSegment{Field: "name"}}
		result, found, err := Extract(nil, segments)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, result)
	})

	t.Run("scalar value with field segment", func(t *testing.T) {
		segments := []PathSegment{FieldSegment{Field: "name"}}
		result, found, err := Extract("string", segments)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, result)
	})

	t.Run("empty object", func(t *testing.T) {
		obj := map[string]any{}
		segments := []PathSegment{FieldSegment{Field: "name"}}
		result, found, err := Extract(obj, segments)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, result)
	})

	t.Run("null value in path", func(t *testing.T) {
		obj := map[string]any{
			"field": nil,
		}
		segments := []PathSegment{
			FieldSegment{Field: "field"},
		}
		result, found, err := Extract(obj, segments)
		require.NoError(t, err)
		assert.True(t, found) // Field exists, value is nil
		assert.Nil(t, result)
	})
}
