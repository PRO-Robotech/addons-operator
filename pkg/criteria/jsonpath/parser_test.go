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

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []PathSegment
		wantErr  bool
	}{
		{
			name:     "empty path",
			path:     "",
			expected: nil,
		},
		{
			name:     "root path",
			path:     "/",
			expected: nil,
		},
		{
			name: "simple field",
			path: "/status",
			expected: []PathSegment{
				FieldSegment{Field: "status"},
			},
		},
		{
			name: "nested fields",
			path: "/status/phase",
			expected: []PathSegment{
				FieldSegment{Field: "status"},
				FieldSegment{Field: "phase"},
			},
		},
		{
			name: "deep nesting",
			path: "/metadata/labels/app",
			expected: []PathSegment{
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "labels"},
				FieldSegment{Field: "app"},
			},
		},
		{
			name: "array index",
			path: "/status/conditions/0",
			expected: []PathSegment{
				FieldSegment{Field: "status"},
				FieldSegment{Field: "conditions"},
				IndexSegment{Index: 0},
			},
		},
		{
			name: "array index in middle",
			path: "/items/0/metadata/name",
			expected: []PathSegment{
				FieldSegment{Field: "items"},
				IndexSegment{Index: 0},
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "name"},
			},
		},
		{
			name: "bracket notation with double quotes",
			path: `/metadata/labels["app.kubernetes.io/name"]`,
			expected: []PathSegment{
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "labels"},
				BracketSegment{Key: "app.kubernetes.io/name"},
			},
		},
		{
			name: "bracket notation with single quotes",
			path: `/metadata/labels['app.kubernetes.io/name']`,
			expected: []PathSegment{
				FieldSegment{Field: "metadata"},
				FieldSegment{Field: "labels"},
				BracketSegment{Key: "app.kubernetes.io/name"},
			},
		},
		{
			name: "bracket index notation",
			path: "/items[0]/name",
			expected: []PathSegment{
				FieldSegment{Field: "items"},
				IndexSegment{Index: 0},
				FieldSegment{Field: "name"},
			},
		},
		{
			name: "filter expression",
			path: `/status/conditions[?(@.type=='Ready')]/status`,
			expected: []PathSegment{
				FieldSegment{Field: "status"},
				FieldSegment{Field: "conditions"},
				FilterSegment{Field: "type", Operator: "==", Value: "Ready"},
				FieldSegment{Field: "status"},
			},
		},
		{
			name: "filter with not equal",
			path: `/items[?(@.name!='default')]`,
			expected: []PathSegment{
				FieldSegment{Field: "items"},
				FilterSegment{Field: "name", Operator: "!=", Value: "default"},
			},
		},
		{
			name:    "path without leading slash",
			path:    "status/phase",
			wantErr: true,
		},
		{
			name:    "unclosed bracket",
			path:    "/labels[app",
			wantErr: true,
		},
		{
			name:    "invalid bracket expression",
			path:    "/labels[invalid]",
			wantErr: true,
		},
		{
			name:    "invalid filter expression",
			path:    `/items[?(@.invalid)]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments, err := Parse(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, segments)
		})
	}
}

func TestFieldSegment_Extract(t *testing.T) {
	tests := []struct {
		name     string
		segment  FieldSegment
		value    any
		expected any
		found    bool
	}{
		{
			name:     "extract existing field",
			segment:  FieldSegment{Field: "name"},
			value:    map[string]any{"name": "test"},
			expected: "test",
			found:    true,
		},
		{
			name:     "extract nested map",
			segment:  FieldSegment{Field: "metadata"},
			value:    map[string]any{"metadata": map[string]any{"name": "test"}},
			expected: map[string]any{"name": "test"},
			found:    true,
		},
		{
			name:     "field not found",
			segment:  FieldSegment{Field: "missing"},
			value:    map[string]any{"name": "test"},
			expected: nil,
			found:    false,
		},
		{
			name:     "not a map",
			segment:  FieldSegment{Field: "name"},
			value:    "string value",
			expected: nil,
			found:    false,
		},
		{
			name:     "nil value",
			segment:  FieldSegment{Field: "name"},
			value:    nil,
			expected: nil,
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := tt.segment.Extract(tt.value)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIndexSegment_Extract(t *testing.T) {
	tests := []struct {
		name     string
		segment  IndexSegment
		value    any
		expected any
		found    bool
	}{
		{
			name:     "extract first element",
			segment:  IndexSegment{Index: 0},
			value:    []any{"first", "second"},
			expected: "first",
			found:    true,
		},
		{
			name:     "extract second element",
			segment:  IndexSegment{Index: 1},
			value:    []any{"first", "second"},
			expected: "second",
			found:    true,
		},
		{
			name:     "index out of bounds",
			segment:  IndexSegment{Index: 5},
			value:    []any{"first", "second"},
			expected: nil,
			found:    false,
		},
		{
			name:     "negative index",
			segment:  IndexSegment{Index: -1},
			value:    []any{"first", "second"},
			expected: nil,
			found:    false,
		},
		{
			name:     "not an array",
			segment:  IndexSegment{Index: 0},
			value:    map[string]any{"name": "test"},
			expected: nil,
			found:    false,
		},
		{
			name:     "nil value",
			segment:  IndexSegment{Index: 0},
			value:    nil,
			expected: nil,
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := tt.segment.Extract(tt.value)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBracketSegment_Extract(t *testing.T) {
	tests := []struct {
		name     string
		segment  BracketSegment
		value    any
		expected any
		found    bool
	}{
		{
			name:     "extract key with special characters",
			segment:  BracketSegment{Key: "app.kubernetes.io/name"},
			value:    map[string]any{"app.kubernetes.io/name": "myapp"},
			expected: "myapp",
			found:    true,
		},
		{
			name:     "key not found",
			segment:  BracketSegment{Key: "missing"},
			value:    map[string]any{"name": "test"},
			expected: nil,
			found:    false,
		},
		{
			name:     "not a map",
			segment:  BracketSegment{Key: "name"},
			value:    []any{"a", "b"},
			expected: nil,
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := tt.segment.Extract(tt.value)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterSegment_Extract(t *testing.T) {
	conditions := []any{
		map[string]any{"type": "Ready", "status": "True"},
		map[string]any{"type": "Progressing", "status": "False"},
		map[string]any{"type": "Available", "status": "True"},
	}

	tests := []struct {
		name     string
		segment  FilterSegment
		value    any
		expected any
		found    bool
	}{
		{
			name:     "filter first match",
			segment:  FilterSegment{Field: "type", Operator: "==", Value: "Ready"},
			value:    conditions,
			expected: map[string]any{"type": "Ready", "status": "True"},
			found:    true,
		},
		{
			name:     "filter middle match",
			segment:  FilterSegment{Field: "type", Operator: "==", Value: "Progressing"},
			value:    conditions,
			expected: map[string]any{"type": "Progressing", "status": "False"},
			found:    true,
		},
		{
			name:     "filter by status",
			segment:  FilterSegment{Field: "status", Operator: "==", Value: "True"},
			value:    conditions,
			expected: map[string]any{"type": "Ready", "status": "True"}, // First match
			found:    true,
		},
		{
			name:     "filter no match",
			segment:  FilterSegment{Field: "type", Operator: "==", Value: "Unknown"},
			value:    conditions,
			expected: nil,
			found:    false,
		},
		{
			name:     "filter not an array",
			segment:  FilterSegment{Field: "type", Operator: "==", Value: "Ready"},
			value:    map[string]any{"type": "Ready"},
			expected: nil,
			found:    false,
		},
		{
			name:     "filter with non-map elements",
			segment:  FilterSegment{Field: "type", Operator: "==", Value: "Ready"},
			value:    []any{"string", 123, nil},
			expected: nil,
			found:    false,
		},
		{
			name:     "filter not equal match",
			segment:  FilterSegment{Field: "type", Operator: "!=", Value: "Ready"},
			value:    conditions,
			expected: map[string]any{"type": "Progressing", "status": "False"},
			found:    true,
		},
		{
			name:    "filter not equal no match",
			segment: FilterSegment{Field: "type", Operator: "!=", Value: "Ready"},
			value: []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
			expected: nil,
			found:    false,
		},
		{
			name:     "filter not equal skips matching",
			segment:  FilterSegment{Field: "status", Operator: "!=", Value: "True"},
			value:    conditions,
			expected: map[string]any{"type": "Progressing", "status": "False"},
			found:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := tt.segment.Extract(tt.value)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindMatchingBracket(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			name:     "simple bracket",
			path:     "[0]",
			expected: 2,
		},
		{
			name:     "quoted content",
			path:     `["app.io/name"]`,
			expected: 14,
		},
		{
			name:     "single quoted",
			path:     `['name']`,
			expected: 7,
		},
		{
			name:     "filter expression",
			path:     `[?(@.type=='Ready')]`,
			expected: 19,
		},
		{
			name:     "nested brackets in quotes",
			path:     `["key[0]"]`,
			expected: 9,
		},
		{
			name:     "unclosed bracket",
			path:     "[open",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findMatchingBracket(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
