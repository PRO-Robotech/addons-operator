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

package dependencies

import (
	"context"
	"testing"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = addonsv1alpha1.AddToScheme(scheme)
	_ = argocdv1alpha1.AddToScheme(scheme)

	return scheme
}

func TestDependencyChecker_NoDependencies(t *testing.T) {
	scheme := setupScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
		},
		Spec: addonsv1alpha1.AddonSpec{
			// No InitDependencies
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.True(t, result.Satisfied)
}

func TestDependencyChecker_ApplicationExists(t *testing.T) {
	scheme := setupScheme()

	// Create existing Application
	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-addon",
			Namespace: "argocd",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(app).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "nonexistent-addon",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
		},
	}

	// Should return satisfied because Application exists (one-time gate)
	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.True(t, result.Satisfied)
}

func TestDependencyChecker_DependencyNotFound(t *testing.T) {
	scheme := setupScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "nonexistent-addon",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.False(t, result.Satisfied)
	assert.Contains(t, result.Reason, "nonexistent-addon not found")
}

func TestDependencyChecker_DependencySatisfied(t *testing.T) {
	scheme := setupScheme()

	// Create dependent addon that is Ready
	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.True(t, result.Satisfied)
}

func TestDependencyChecker_DependencyNotSatisfied(t *testing.T) {
	scheme := setupScheme()

	// Create dependent addon that is NOT Ready
	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 2,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.False(t, result.Satisfied)
	assert.Contains(t, result.Reason, "Waiting for cert-manager")
	assert.Contains(t, result.Reason, ".status.observedGeneration")
}

func TestDependencyChecker_MultipleDependencies(t *testing.T) {
	scheme := setupScheme()

	// Create two dependent addons, one ready, one not
	depAddon1 := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	depAddon2 := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prometheus",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 2,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon1, depAddon2).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
				{
					Name: "prometheus",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.False(t, result.Satisfied)
	assert.Contains(t, result.Reason, "prometheus")
}

func TestDependencyChecker_ExistsOperator(t *testing.T) {
	scheme := setupScheme()

	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorExists,
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.True(t, result.Satisfied)
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		operator addonsv1alpha1.CriterionOperator
		expected *apiextensionsv1.JSON
		match    bool
	}{
		{
			name:     "equal strings",
			actual:   "Ready",
			operator: addonsv1alpha1.OperatorEqual,
			expected: jsonValue("Ready"),
			match:    true,
		},
		{
			name:     "not equal strings",
			actual:   "Pending",
			operator: addonsv1alpha1.OperatorEqual,
			expected: jsonValue("Ready"),
			match:    false,
		},
		{
			name:     "not equal operator",
			actual:   "Pending",
			operator: addonsv1alpha1.OperatorNotEqual,
			expected: jsonValue("Ready"),
			match:    true,
		},
		{
			name:     "exists when present",
			actual:   "something",
			operator: addonsv1alpha1.OperatorExists,
			expected: nil,
			match:    true,
		},
		{
			name:     "exists when nil",
			actual:   nil,
			operator: addonsv1alpha1.OperatorExists,
			expected: nil,
			match:    false,
		},
		{
			name:     "not exists when nil",
			actual:   nil,
			operator: addonsv1alpha1.OperatorNotExists,
			expected: nil,
			match:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compareValues(tt.actual, tt.operator, tt.expected)
			require.NoError(t, err)
			assert.Equal(t, tt.match, result)
		})
	}
}

func TestDependencyChecker_PathNotFound(t *testing.T) {
	scheme := setupScheme()

	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
		// No status set
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.False(t, result.Satisfied)
	assert.Contains(t, result.Reason, "not found")
}

func TestDependencyChecker_MultipleCriteria(t *testing.T) {
	scheme := setupScheme()

	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorExists,
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.True(t, result.Satisfied)
}

func TestCompareValues_UnsupportedOperator(t *testing.T) {
	_, err := compareValues("value", "InvalidOperator", jsonValue("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operator")
}

func TestDependencyChecker_WithSource(t *testing.T) {
	scheme := setupScheme()

	// Create a dependent addon
	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
	}

	// Create a secret that we'll check
	secret := &unstructured.Unstructured{}
	secret.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})
	secret.SetName("my-secret")
	secret.SetNamespace("default")
	secret.Object["data"] = map[string]any{
		"key": "value",
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	// Add secret manually since it needs unstructured
	err := fakeClient.Create(context.Background(), secret)
	require.NoError(t, err)

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							Source: &addonsv1alpha1.CriterionSource{
								APIVersion: "v1",
								Kind:       "Secret",
								Name:       "my-secret",
								Namespace:  "default", // Explicit namespace required
							},
							JSONPath: "$.data.key",
							Operator: addonsv1alpha1.OperatorExists,
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.True(t, result.Satisfied)
}

func TestDependencyChecker_SourceNotFound(t *testing.T) {
	scheme := setupScheme()

	depAddon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(depAddon).
		Build()

	checker := NewDependencyChecker(fakeClient)

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio",
		},
		Spec: addonsv1alpha1.AddonSpec{
			InitDependencies: []addonsv1alpha1.Dependency{
				{
					Name: "cert-manager",
					Criteria: []addonsv1alpha1.Criterion{
						{
							Source: &addonsv1alpha1.CriterionSource{
								APIVersion: "v1",
								Kind:       "Secret",
								Name:       "nonexistent-secret",
							},
							JSONPath: "$.data.key",
							Operator: addonsv1alpha1.OperatorExists,
						},
					},
				},
			},
		},
	}

	result, err := checker.CheckDependencies(context.Background(), addon, "argocd")
	require.NoError(t, err)
	assert.False(t, result.Satisfied)
	assert.Contains(t, result.Reason, "not found")
}

// Helper to create JSON value
func jsonValue(v string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(`"` + v + `"`)}
}
