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

package criteria

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// Helper to create JSON value
func jsonValue(s string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(s)}
}

func TestEvalEqual(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
		wantErr  bool
	}{
		{
			name:     "string equal",
			actual:   "Running",
			expected: jsonValue(`"Running"`),
			want:     true,
		},
		{
			name:     "string not equal",
			actual:   "Running",
			expected: jsonValue(`"Pending"`),
			want:     false,
		},
		{
			name:     "number equal",
			actual:   float64(42),
			expected: jsonValue(`42`),
			want:     true,
		},
		{
			name:     "number equal int vs float",
			actual:   42,
			expected: jsonValue(`42`),
			want:     true,
		},
		{
			name:     "boolean equal true",
			actual:   true,
			expected: jsonValue(`true`),
			want:     true,
		},
		{
			name:     "boolean equal false",
			actual:   false,
			expected: jsonValue(`false`),
			want:     true,
		},
		{
			name:     "nil equal nil",
			actual:   nil,
			expected: nil,
			want:     true,
		},
		{
			name:     "nil vs value",
			actual:   nil,
			expected: jsonValue(`"value"`),
			want:     false,
		},
		{
			name:     "value vs nil",
			actual:   "value",
			expected: nil,
			want:     false,
		},
		{
			name:     "empty expected",
			actual:   nil,
			expected: &apiextensionsv1.JSON{Raw: []byte{}},
			want:     true,
		},
		{
			name:     "array equal",
			actual:   []any{"a", "b"},
			expected: jsonValue(`["a", "b"]`),
			want:     true,
		},
		{
			name:     "map equal",
			actual:   map[string]any{"key": "value"},
			expected: jsonValue(`{"key": "value"}`),
			want:     true,
		},
		{
			name:     "string vs number type coercion",
			actual:   "42",
			expected: jsonValue(`42`),
			want:     true, // Type coercion via string comparison
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalEqual(tt.actual, tt.expected)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalNotEqual(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
	}{
		{
			name:     "string not equal",
			actual:   "Running",
			expected: jsonValue(`"Pending"`),
			want:     true,
		},
		{
			name:     "string equal",
			actual:   "Running",
			expected: jsonValue(`"Running"`),
			want:     false,
		},
		{
			name:     "number not equal",
			actual:   float64(42),
			expected: jsonValue(`99`),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalNotEqual(tt.actual, tt.expected)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalIn(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
		wantErr  bool
	}{
		{
			name:     "string in list",
			actual:   "Running",
			expected: jsonValue(`["Running", "Pending", "Succeeded"]`),
			want:     true,
		},
		{
			name:     "string not in list",
			actual:   "Failed",
			expected: jsonValue(`["Running", "Pending", "Succeeded"]`),
			want:     false,
		},
		{
			name:     "number in list",
			actual:   float64(42),
			expected: jsonValue(`[1, 42, 100]`),
			want:     true,
		},
		{
			name:     "empty list",
			actual:   "value",
			expected: jsonValue(`[]`),
			want:     false,
		},
		{
			name:     "nil expected",
			actual:   "value",
			expected: nil,
			want:     false,
		},
		{
			name:     "not an array",
			actual:   "value",
			expected: jsonValue(`"not-an-array"`),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalIn(tt.actual, tt.expected)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalNotIn(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
	}{
		{
			name:     "string not in list",
			actual:   "Failed",
			expected: jsonValue(`["Running", "Pending"]`),
			want:     true,
		},
		{
			name:     "string in list",
			actual:   "Running",
			expected: jsonValue(`["Running", "Pending"]`),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalNotIn(tt.actual, tt.expected)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalGreaterThan(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
		wantErr  bool
	}{
		{
			name:     "float64 greater",
			actual:   float64(10),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "float64 equal",
			actual:   float64(5),
			expected: jsonValue(`5`),
			want:     false,
		},
		{
			name:     "float64 less",
			actual:   float64(3),
			expected: jsonValue(`5`),
			want:     false,
		},
		{
			name:     "int greater",
			actual:   10,
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "int32",
			actual:   int32(10),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "int64",
			actual:   int64(10),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "float32",
			actual:   float32(10),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "string number",
			actual:   "10",
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "non-numeric actual",
			actual:   "not-a-number",
			expected: jsonValue(`5`),
			want:     false, // Type mismatch = false
		},
		{
			name:     "nil expected",
			actual:   float64(10),
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "non-numeric expected",
			actual:   float64(10),
			expected: jsonValue(`"not-a-number"`),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalGreaterThan(tt.actual, tt.expected)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalGreaterOrEqual(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
	}{
		{
			name:     "greater",
			actual:   float64(10),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "equal",
			actual:   float64(5),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "less",
			actual:   float64(3),
			expected: jsonValue(`5`),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalGreaterOrEqual(tt.actual, tt.expected)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalLessThan(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
	}{
		{
			name:     "less",
			actual:   float64(3),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "equal",
			actual:   float64(5),
			expected: jsonValue(`5`),
			want:     false,
		},
		{
			name:     "greater",
			actual:   float64(10),
			expected: jsonValue(`5`),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalLessThan(tt.actual, tt.expected)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalLessOrEqual(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
	}{
		{
			name:     "less",
			actual:   float64(3),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "equal",
			actual:   float64(5),
			expected: jsonValue(`5`),
			want:     true,
		},
		{
			name:     "greater",
			actual:   float64(10),
			expected: jsonValue(`5`),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalLessOrEqual(tt.actual, tt.expected)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvalExists(t *testing.T) {
	assert.True(t, EvalExists(true))
	assert.False(t, EvalExists(false))
}

func TestEvalNotExists(t *testing.T) {
	assert.True(t, EvalNotExists(false))
	assert.False(t, EvalNotExists(true))
}

func TestEvalMatches(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected *apiextensionsv1.JSON
		want     bool
		wantErr  bool
	}{
		{
			name:     "simple match",
			actual:   "hello-world",
			expected: jsonValue(`"hello-.*"`),
			want:     true,
		},
		{
			name:     "no match",
			actual:   "goodbye",
			expected: jsonValue(`"hello-.*"`),
			want:     false,
		},
		{
			name:     "exact match",
			actual:   "test",
			expected: jsonValue(`"^test$"`),
			want:     true,
		},
		{
			name:     "partial match",
			actual:   "testing",
			expected: jsonValue(`"test"`),
			want:     true,
		},
		{
			name:     "digit pattern",
			actual:   "pod-123",
			expected: jsonValue(`"pod-\\d+"`),
			want:     true,
		},
		{
			name:     "non-string actual",
			actual:   123,
			expected: jsonValue(`"\\d+"`),
			want:     false, // Type mismatch
		},
		{
			name:     "invalid regex",
			actual:   "test",
			expected: jsonValue(`"[invalid"`),
			wantErr:  true,
		},
		{
			name:     "empty pattern",
			actual:   "anything",
			expected: jsonValue(`""`),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalMatches(tt.actual, tt.expected)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  float64
		ok    bool
	}{
		{"float64", float64(1.5), 1.5, true},
		{"float32", float32(1.5), 1.5, true},
		{"int", 42, 42.0, true},
		{"int32", int32(42), 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"string number", "42.5", 42.5, true},
		{"string non-number", "not-a-number", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
		{"array", []any{1, 2}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.value)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.InDelta(t, tt.want, got, 0.001)
			}
		})
	}
}

func TestDeepEqual(t *testing.T) {
	tests := []struct {
		name string
		a    any
		b    any
		want bool
	}{
		{"same strings", "test", "test", true},
		{"different strings", "a", "b", false},
		{"int vs float64", 42, float64(42), true},
		{"string coercion", "42", 42, true},
		{"nil vs nil", nil, nil, true},
		{"same arrays", []any{1, 2}, []any{1, 2}, true},
		{"different arrays", []any{1, 2}, []any{2, 1}, false},
		{"same maps", map[string]any{"a": 1}, map[string]any{"a": 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deepEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}
