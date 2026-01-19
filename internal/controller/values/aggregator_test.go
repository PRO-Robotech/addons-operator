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

package values

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// Helper to create runtime.RawExtension from map
func rawExtension(v any) runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return runtime.RawExtension{Raw: data}
}

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		base     map[string]any
		overlay  map[string]any
		expected map[string]any
	}{
		{
			name:     "nil base",
			base:     nil,
			overlay:  map[string]any{"key": "value"},
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "nil overlay",
			base:     map[string]any{"key": "value"},
			overlay:  nil,
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "scalar replacement",
			base:     map[string]any{"key": "old"},
			overlay:  map[string]any{"key": "new"},
			expected: map[string]any{"key": "new"},
		},
		{
			name:     "add new key",
			base:     map[string]any{"existing": "value"},
			overlay:  map[string]any{"new": "value"},
			expected: map[string]any{"existing": "value", "new": "value"},
		},
		{
			name: "nested object merge",
			base: map[string]any{
				"tls": map[string]any{
					"enabled": false,
					"mode":    "permissive",
				},
			},
			overlay: map[string]any{
				"tls": map[string]any{
					"enabled": true,
					"ca":      "xxx",
				},
			},
			expected: map[string]any{
				"tls": map[string]any{
					"enabled": true,
					"mode":    "permissive",
					"ca":      "xxx",
				},
			},
		},
		{
			name: "array replacement",
			base: map[string]any{
				"items": []any{"a", "b"},
			},
			overlay: map[string]any{
				"items": []any{"c", "d", "e"},
			},
			expected: map[string]any{
				"items": []any{"c", "d", "e"},
			},
		},
		{
			name: "deep nested merge",
			base: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			overlay: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"key2": "overridden",
						"key3": "new",
					},
				},
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"key1": "value1",
						"key2": "overridden",
						"key3": "new",
					},
				},
			},
		},
		{
			name: "mixed types - overlay wins",
			base: map[string]any{
				"config": map[string]any{"nested": "value"},
			},
			overlay: map[string]any{
				"config": "simple string",
			},
			expected: map[string]any{
				"config": "simple string",
			},
		},
		{
			name: "priority merge with nested override",
			base: map[string]any{
				"ipam": map[string]any{"mode": "kubernetes"},
				"tls":  map[string]any{"enabled": false},
			},
			overlay: map[string]any{
				"tls": map[string]any{
					"enabled": true,
					"ca":      "xxx",
				},
			},
			expected: map[string]any{
				"ipam": map[string]any{"mode": "kubernetes"},
				"tls": map[string]any{
					"enabled": true,
					"ca":      "xxx",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepMerge(tt.base, tt.overlay)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUnmarshalValues(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected map[string]any
		wantErr  bool
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: map[string]any{},
			wantErr:  false,
		},
		{
			name:     "valid JSON",
			input:    []byte(`{"key": "value", "number": 42}`),
			expected: map[string]any{"key": "value", "number": float64(42)},
			wantErr:  false,
		},
		{
			name:    "invalid JSON",
			input:   []byte(`{invalid}`),
			wantErr: true,
		},
		{
			name:  "nested JSON",
			input: []byte(`{"outer": {"inner": "value"}}`),
			expected: map[string]any{
				"outer": map[string]any{"inner": "value"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UnmarshalValues(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAggregator_AggregateValues(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	// Create test AddonValues with different labels and priorities
	baseValues := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cilium-base",
			Labels: map[string]string{
				"addon": "cilium",
				"tier":  "base",
			},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{
				"ipam": map[string]any{"mode": "kubernetes"},
				"tls":  map[string]any{"enabled": false},
			}),
		},
	}

	certValues := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cilium-certs",
			Labels: map[string]string{
				"addon": "cilium",
				"tier":  "certs",
			},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{
				"tls": map[string]any{
					"enabled": true,
					"ca":      "xxx",
				},
			}),
		},
	}

	immutableValues := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cilium-immutable",
			Labels: map[string]string{
				"addon": "cilium",
				"tier":  "immutable",
			},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{
				"tls": map[string]any{
					"enforceMode": "strict",
				},
			}),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(baseValues, certValues, immutableValues).
		Build()

	aggregator := NewAggregator(fakeClient)

	// Create Addon with selectors at different priorities
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cilium",
		},
		Spec: addonsv1alpha1.AddonSpec{
			ValuesSelectors: []addonsv1alpha1.ValuesSelector{
				{
					Name:        "base",
					Priority:    0,
					MatchLabels: map[string]string{"addon": "cilium", "tier": "base"},
				},
				{
					Name:        "immutable",
					Priority:    99,
					MatchLabels: map[string]string{"addon": "cilium", "tier": "immutable"},
				},
			},
		},
		Status: addonsv1alpha1.AddonStatus{
			PhaseValuesSelector: []addonsv1alpha1.ValuesSelector{
				{
					Name:        "certs",
					Priority:    20,
					MatchLabels: map[string]string{"addon": "cilium", "tier": "certs"},
				},
			},
		},
	}

	result, err := aggregator.AggregateValues(context.Background(), addon)
	require.NoError(t, err)

	// Verify the priority-based merge result
	expected := map[string]any{
		"ipam": map[string]any{"mode": "kubernetes"},
		"tls": map[string]any{
			"enabled":     true,     // from priority 20
			"ca":          "xxx",    // from priority 20
			"enforceMode": "strict", // from priority 99
		},
	}

	assert.Equal(t, expected, result)
}

func TestAggregator_EmptySelectors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	aggregator := NewAggregator(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: addonsv1alpha1.AddonSpec{},
	}

	result, err := aggregator.AggregateValues(context.Background(), addon)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, result)
}

func TestAggregator_NoMatchingValues(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	aggregator := NewAggregator(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: addonsv1alpha1.AddonSpec{
			ValuesSelectors: []addonsv1alpha1.ValuesSelector{
				{
					Name:        "nonexistent",
					Priority:    0,
					MatchLabels: map[string]string{"addon": "nonexistent"},
				},
			},
		},
	}

	result, err := aggregator.AggregateValues(context.Background(), addon)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, result)
}

func TestAggregator_SortByName(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	// Create AddonValues with same labels but different names
	// They should be merged in alphabetical order
	valueA := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "b-value", // alphabetically second
			Labels: map[string]string{"app": "test"},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{"key": "from-b"}),
		},
	}

	valueB := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "a-value", // alphabetically first
			Labels: map[string]string{"app": "test"},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{"key": "from-a"}),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(valueA, valueB).
		Build()

	aggregator := NewAggregator(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: addonsv1alpha1.AddonSpec{
			ValuesSelectors: []addonsv1alpha1.ValuesSelector{
				{
					Name:        "all",
					Priority:    0,
					MatchLabels: map[string]string{"app": "test"},
				},
			},
		},
	}

	result, err := aggregator.AggregateValues(context.Background(), addon)
	require.NoError(t, err)

	// a-value is processed first, b-value second (alphabetical)
	// So b-value's "key" should override a-value's "key"
	assert.Equal(t, "from-b", result["key"])
}

func TestComputeHash(t *testing.T) {
	values1 := map[string]any{"key": "value"}
	values2 := map[string]any{"key": "different"}
	values3 := map[string]any{"key": "value"}

	hash1, err := ComputeHash(values1)
	require.NoError(t, err)

	hash2, err := ComputeHash(values2)
	require.NoError(t, err)

	hash3, err := ComputeHash(values3)
	require.NoError(t, err)

	// Same values should produce same hash
	assert.Equal(t, hash1, hash3)

	// Different values should produce different hash
	assert.NotEqual(t, hash1, hash2)
}

func TestComputeHash_Error(t *testing.T) {
	// Test with unmarshalable value (channel cannot be JSON serialized)
	ch := make(chan int)
	values := map[string]any{"channel": ch}

	hash, err := ComputeHash(values)
	assert.Error(t, err)
	assert.Empty(t, hash)
	assert.Contains(t, err.Error(), "marshal values")
}

// Tests for exact match labels (only addons.in-cloud.io/ prefix labels are compared)

func TestFilterAddonLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected map[string]string
	}{
		{
			name:     "nil labels",
			labels:   nil,
			expected: map[string]string{},
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "only addon labels",
			labels: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"addons.in-cloud.io/layer": "base",
			},
			expected: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"addons.in-cloud.io/layer": "base",
			},
		},
		{
			name: "only system labels",
			labels: map[string]string{
				"kubernetes.io/name":     "test",
				"app.kubernetes.io/name": "my-app",
			},
			expected: map[string]string{},
		},
		{
			name: "mixed labels",
			labels: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"kubernetes.io/name":       "test",
				"custom-label":             "value",
			},
			expected: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterAddonLabels(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExactMatchAddonLabels(t *testing.T) {
	tests := []struct {
		name     string
		selector map[string]string
		resource map[string]string
		expected bool
	}{
		{
			name:     "both empty",
			selector: map[string]string{},
			resource: map[string]string{},
			expected: true,
		},
		{
			name: "exact match single label",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
			expected: true,
		},
		{
			name: "exact match multiple labels",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"addons.in-cloud.io/layer": "base",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"addons.in-cloud.io/layer": "base",
			},
			expected: true,
		},
		{
			name: "resource has extra addon label - NO MATCH",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"addons.in-cloud.io/layer": "feature",
			},
			expected: false,
		},
		{
			name: "selector has more labels than resource - NO MATCH",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"addons.in-cloud.io/layer": "base",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
			expected: false,
		},
		{
			name: "different values - NO MATCH",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "podinfo",
			},
			expected: false,
		},
		{
			name: "system labels ignored - MATCH",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"kubernetes.io/name":       "test",
				"app.kubernetes.io/name":   "my-app",
			},
			expected: true,
		},
		{
			name: "system labels in selector ignored - MATCH",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"kubernetes.io/name":       "different",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"kubernetes.io/name":       "test",
			},
			expected: true,
		},
		{
			name: "non-prefixed labels ignored - MATCH",
			selector: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"custom":                   "label",
			},
			resource: map[string]string{
				"addons.in-cloud.io/addon": "cilium",
				"other":                    "value",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExactMatchAddonLabels(tt.selector, tt.resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAggregator_ExactMatchLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	// Create AddonValues with addon-prefixed labels
	baseOnly := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-base",
			Labels: map[string]string{
				"addons.in-cloud.io/addon": "test",
			},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{"source": "base"}),
		},
	}

	baseWithFeature := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-feature",
			Labels: map[string]string{
				"addons.in-cloud.io/addon": "test",
				"addons.in-cloud.io/layer": "feature",
			},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: rawExtension(map[string]any{"source": "feature"}),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(baseOnly, baseWithFeature).
		Build()

	aggregator := NewAggregator(fakeClient)

	t.Run("single label selector matches only single label resource", func(t *testing.T) {
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: addonsv1alpha1.AddonSpec{
				ValuesSelectors: []addonsv1alpha1.ValuesSelector{
					{
						Name:     "default",
						Priority: 0,
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": "test",
						},
					},
				},
			},
		}

		result, err := aggregator.AggregateValues(context.Background(), addon)
		require.NoError(t, err)

		// Should only select test-base (single addon label), not test-feature
		assert.Equal(t, "base", result["source"])
	})

	t.Run("two label selector matches only two label resource", func(t *testing.T) {
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: addonsv1alpha1.AddonSpec{
				ValuesSelectors: []addonsv1alpha1.ValuesSelector{
					{
						Name:     "feature",
						Priority: 0,
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": "test",
							"addons.in-cloud.io/layer": "feature",
						},
					},
				},
			},
		}

		result, err := aggregator.AggregateValues(context.Background(), addon)
		require.NoError(t, err)

		// Should only select test-feature (two addon labels), not test-base
		assert.Equal(t, "feature", result["source"])
	})

	t.Run("both selectors select their respective resources", func(t *testing.T) {
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: addonsv1alpha1.AddonSpec{
				ValuesSelectors: []addonsv1alpha1.ValuesSelector{
					{
						Name:     "default",
						Priority: 0,
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": "test",
						},
					},
					{
						Name:     "feature",
						Priority: 10,
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": "test",
							"addons.in-cloud.io/layer": "feature",
						},
					},
				},
			},
		}

		result, err := aggregator.AggregateValues(context.Background(), addon)
		require.NoError(t, err)

		// Priority 0 (base) processed first, priority 10 (feature) second
		// Feature should override base
		assert.Equal(t, "feature", result["source"])
	})
}
