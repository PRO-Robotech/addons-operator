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
func jsonValueEval(s string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(s)}
}

func TestNewEvaluator(t *testing.T) {
	e := NewEvaluator()
	require.NotNil(t, e)
	require.NotNil(t, e.pathCache)
}

func TestEvaluator_Evaluate(t *testing.T) {
	e := NewEvaluator()

	obj := map[string]any{
		"status": map[string]any{
			"phase":    "Running",
			"replicas": float64(3),
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
		"metadata": map[string]any{
			"name": "test-pod",
			"labels": map[string]any{
				"app":                    "myapp",
				"app.kubernetes.io/name": "myapp",
			},
		},
	}

	tests := []struct {
		name      string
		path      string
		operator  Operator
		expected  *apiextensionsv1.JSON
		satisfied bool
		hasError  bool
	}{
		{
			name:      "equal satisfied",
			path:      "/status/phase",
			operator:  OperatorEqual,
			expected:  jsonValueEval(`"Running"`),
			satisfied: true,
		},
		{
			name:      "equal not satisfied",
			path:      "/status/phase",
			operator:  OperatorEqual,
			expected:  jsonValueEval(`"Pending"`),
			satisfied: false,
		},
		{
			name:      "not equal satisfied",
			path:      "/status/phase",
			operator:  OperatorNotEqual,
			expected:  jsonValueEval(`"Pending"`),
			satisfied: true,
		},
		{
			name:      "greater than satisfied",
			path:      "/status/replicas",
			operator:  OperatorGreaterThan,
			expected:  jsonValueEval(`2`),
			satisfied: true,
		},
		{
			name:      "greater than not satisfied",
			path:      "/status/replicas",
			operator:  OperatorGreaterThan,
			expected:  jsonValueEval(`5`),
			satisfied: false,
		},
		{
			name:      "greater or equal satisfied (equal)",
			path:      "/status/replicas",
			operator:  OperatorGreaterOrEqual,
			expected:  jsonValueEval(`3`),
			satisfied: true,
		},
		{
			name:      "less than satisfied",
			path:      "/status/replicas",
			operator:  OperatorLessThan,
			expected:  jsonValueEval(`5`),
			satisfied: true,
		},
		{
			name:      "less or equal satisfied (equal)",
			path:      "/status/replicas",
			operator:  OperatorLessOrEqual,
			expected:  jsonValueEval(`3`),
			satisfied: true,
		},
		{
			name:      "in satisfied",
			path:      "/status/phase",
			operator:  OperatorIn,
			expected:  jsonValueEval(`["Running", "Pending", "Succeeded"]`),
			satisfied: true,
		},
		{
			name:      "in not satisfied",
			path:      "/status/phase",
			operator:  OperatorIn,
			expected:  jsonValueEval(`["Pending", "Failed"]`),
			satisfied: false,
		},
		{
			name:      "not in satisfied",
			path:      "/status/phase",
			operator:  OperatorNotIn,
			expected:  jsonValueEval(`["Pending", "Failed"]`),
			satisfied: true,
		},
		{
			name:      "exists satisfied",
			path:      "/status/phase",
			operator:  OperatorExists,
			expected:  nil,
			satisfied: true,
		},
		{
			name:      "exists not satisfied",
			path:      "/status/nonexistent",
			operator:  OperatorExists,
			expected:  nil,
			satisfied: false,
		},
		{
			name:      "not exists satisfied",
			path:      "/status/nonexistent",
			operator:  OperatorNotExists,
			expected:  nil,
			satisfied: true,
		},
		{
			name:      "not exists not satisfied",
			path:      "/status/phase",
			operator:  OperatorNotExists,
			expected:  nil,
			satisfied: false,
		},
		{
			name:      "matches satisfied",
			path:      "/metadata/name",
			operator:  OperatorMatches,
			expected:  jsonValueEval(`"test-.*"`),
			satisfied: true,
		},
		{
			name:      "matches not satisfied",
			path:      "/metadata/name",
			operator:  OperatorMatches,
			expected:  jsonValueEval(`"prod-.*"`),
			satisfied: false,
		},
		{
			name:      "bracket notation",
			path:      `/metadata/labels["app.kubernetes.io/name"]`,
			operator:  OperatorEqual,
			expected:  jsonValueEval(`"myapp"`),
			satisfied: true,
		},
		{
			name:      "filter expression",
			path:      `/status/conditions[?(@.type=='Ready')]/status`,
			operator:  OperatorEqual,
			expected:  jsonValueEval(`"True"`),
			satisfied: true,
		},
		{
			name:      "path not found for comparison",
			path:      "/nonexistent/path",
			operator:  OperatorEqual,
			expected:  jsonValueEval(`"value"`),
			satisfied: false,
		},
		{
			name:     "invalid path",
			path:     "no-leading-slash",
			operator: OperatorEqual,
			expected: jsonValueEval(`"value"`),
			hasError: true,
		},
		{
			name:     "unknown operator",
			path:     "/status/phase",
			operator: "UnknownOp",
			expected: jsonValueEval(`"value"`),
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Evaluate(obj, tt.path, tt.operator, tt.expected)
			if tt.hasError {
				assert.NotNil(t, result.Error)
				return
			}
			assert.Nil(t, result.Error)
			assert.Equal(t, tt.satisfied, result.Satisfied)
			assert.NotEmpty(t, result.Reason)
		})
	}
}

func TestEvaluator_EvaluateAll(t *testing.T) {
	e := NewEvaluator()

	obj := map[string]any{
		"status": map[string]any{
			"phase":    "Running",
			"replicas": float64(3),
		},
	}

	t.Run("all satisfied", func(t *testing.T) {
		criteria := []CriterionInput{
			{Path: "/status/phase", Operator: OperatorEqual, Expected: jsonValueEval(`"Running"`)},
			{Path: "/status/replicas", Operator: OperatorGreaterThan, Expected: jsonValueEval(`2`)},
		}

		allSatisfied, results := e.EvaluateAll(obj, criteria)
		assert.True(t, allSatisfied)
		assert.Len(t, results, 2)
		assert.True(t, results[0].Satisfied)
		assert.True(t, results[1].Satisfied)
	})

	t.Run("one not satisfied", func(t *testing.T) {
		criteria := []CriterionInput{
			{Path: "/status/phase", Operator: OperatorEqual, Expected: jsonValueEval(`"Running"`)},
			{Path: "/status/replicas", Operator: OperatorGreaterThan, Expected: jsonValueEval(`5`)},
		}

		allSatisfied, results := e.EvaluateAll(obj, criteria)
		assert.False(t, allSatisfied)
		assert.Len(t, results, 2)
		assert.True(t, results[0].Satisfied)
		assert.False(t, results[1].Satisfied)
	})

	t.Run("empty criteria", func(t *testing.T) {
		allSatisfied, results := e.EvaluateAll(obj, nil)
		assert.True(t, allSatisfied)
		assert.Empty(t, results)
	})
}

func TestEvaluator_PathCaching(t *testing.T) {
	e := NewEvaluator()
	obj := map[string]any{"status": map[string]any{"phase": "Running"}}

	// First call should populate cache
	_ = e.Evaluate(obj, "/status/phase", OperatorEqual, jsonValueEval(`"Running"`))
	assert.Len(t, e.pathCache, 1)

	// Second call should use cache
	_ = e.Evaluate(obj, "/status/phase", OperatorEqual, jsonValueEval(`"Running"`))
	assert.Len(t, e.pathCache, 1) // Still 1

	// Different path should add to cache
	_ = e.Evaluate(obj, "/status/replicas", OperatorExists, nil)
	assert.Len(t, e.pathCache, 2)
}

func TestEvaluator_ErrorCases(t *testing.T) {
	e := NewEvaluator()
	obj := map[string]any{"value": "test"}

	t.Run("invalid regex pattern", func(t *testing.T) {
		result := e.Evaluate(obj, "/value", OperatorMatches, jsonValueEval(`"[invalid"`))
		assert.NotNil(t, result.Error)
		assert.False(t, result.Satisfied)
		assert.Contains(t, result.Reason, "evaluation error")
	})

	t.Run("in operator with non-array", func(t *testing.T) {
		result := e.Evaluate(obj, "/value", OperatorIn, jsonValueEval(`"not-an-array"`))
		assert.NotNil(t, result.Error)
		assert.False(t, result.Satisfied)
	})

	t.Run("numeric comparison with non-number expected", func(t *testing.T) {
		obj := map[string]any{"count": float64(5)}
		result := e.Evaluate(obj, "/count", OperatorGreaterThan, jsonValueEval(`"not-a-number"`))
		assert.NotNil(t, result.Error)
		assert.False(t, result.Satisfied)
	})
}

func TestEvaluator_ComplexScenarios(t *testing.T) {
	e := NewEvaluator()

	// Kubernetes Pod-like object
	pod := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "nginx-abc123",
			"namespace": "production",
			"labels": map[string]any{
				"app":                       "nginx",
				"environment":               "prod",
				"app.kubernetes.io/name":    "nginx",
				"app.kubernetes.io/version": "1.19",
			},
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "nginx",
					"image": "nginx:1.19",
				},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{"type": "Initialized", "status": "True"},
				map[string]any{"type": "Ready", "status": "True"},
				map[string]any{"type": "ContainersReady", "status": "True"},
				map[string]any{"type": "PodScheduled", "status": "True"},
			},
		},
	}

	t.Run("ready condition check", func(t *testing.T) {
		result := e.Evaluate(
			pod,
			`/status/conditions[?(@.type=='Ready')]/status`,
			OperatorEqual,
			jsonValueEval(`"True"`),
		)
		assert.True(t, result.Satisfied)
	})

	t.Run("label with special characters", func(t *testing.T) {
		result := e.Evaluate(
			pod,
			`/metadata/labels["app.kubernetes.io/name"]`,
			OperatorEqual,
			jsonValueEval(`"nginx"`),
		)
		assert.True(t, result.Satisfied)
	})

	t.Run("name pattern matching", func(t *testing.T) {
		result := e.Evaluate(
			pod,
			"/metadata/name",
			OperatorMatches,
			jsonValueEval(`"nginx-[a-z0-9]+"`),
		)
		assert.True(t, result.Satisfied)
	})

	t.Run("namespace in allowed list", func(t *testing.T) {
		result := e.Evaluate(
			pod,
			"/metadata/namespace",
			OperatorIn,
			jsonValueEval(`["production", "staging"]`),
		)
		assert.True(t, result.Satisfied)
	})

	t.Run("complex multi-criteria", func(t *testing.T) {
		criteria := []CriterionInput{
			{Path: "/status/phase", Operator: OperatorEqual, Expected: jsonValueEval(`"Running"`)},
			{Path: `/status/conditions[?(@.type=='Ready')]/status`, Operator: OperatorEqual, Expected: jsonValueEval(`"True"`)},
			{Path: `/metadata/labels["app.kubernetes.io/name"]`, Operator: OperatorEqual, Expected: jsonValueEval(`"nginx"`)},
			{Path: "/metadata/namespace", Operator: OperatorIn, Expected: jsonValueEval(`["production", "staging"]`)},
		}

		allSatisfied, results := e.EvaluateAll(pod, criteria)
		assert.True(t, allSatisfied)
		for _, r := range results {
			assert.True(t, r.Satisfied, "criterion should be satisfied: %s", r.Reason)
		}
	})
}

// Benchmark tests
func BenchmarkEvaluator_SimpleField(b *testing.B) {
	e := NewEvaluator()
	obj := map[string]any{"status": map[string]any{"phase": "Running"}}
	expected := jsonValueEval(`"Running"`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(obj, "/status/phase", OperatorEqual, expected)
	}
}

func BenchmarkEvaluator_DeepNesting(b *testing.B) {
	e := NewEvaluator()
	obj := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": map[string]any{
						"e": "value",
					},
				},
			},
		},
	}
	expected := jsonValueEval(`"value"`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(obj, "/a/b/c/d/e", OperatorEqual, expected)
	}
}

func BenchmarkEvaluator_FilterExpression(b *testing.B) {
	e := NewEvaluator()
	obj := map[string]any{
		"conditions": []any{
			map[string]any{"type": "A", "status": "True"},
			map[string]any{"type": "B", "status": "True"},
			map[string]any{"type": "Ready", "status": "True"},
			map[string]any{"type": "D", "status": "True"},
		},
	}
	expected := jsonValueEval(`"True"`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(obj, `/conditions[?(@.type=='Ready')]/status`, OperatorEqual, expected)
	}
}

func BenchmarkEvaluator_PathCaching(b *testing.B) {
	e := NewEvaluator()
	obj := map[string]any{"status": map[string]any{"phase": "Running"}}
	expected := jsonValueEval(`"Running"`)

	// Warm up cache
	e.Evaluate(obj, "/status/phase", OperatorEqual, expected)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(obj, "/status/phase", OperatorEqual, expected)
	}
}
