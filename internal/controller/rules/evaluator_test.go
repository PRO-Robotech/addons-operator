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

package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func TestRuleEvaluator_NoCriteria(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name:     "always-active",
					Criteria: nil, // No criteria
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "default",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "base"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Equal(t, "No conditions", ruleStatuses[0].Message)
	assert.Len(t, activeSelectors, 1)
	assert.Equal(t, "default", activeSelectors[0].Name)
}

func TestRuleEvaluator_CriteriaOnTargetAddon(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-ready",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "certificates",
						Priority:    20,
						MatchLabels: map[string]string{"feature": "certs"},
					},
				},
			},
		},
	}

	// Target addon with Ready status
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_CriteriaNotMet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-ready",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "certificates",
						Priority:    20,
						MatchLabels: map[string]string{"feature": "certs"},
					},
				},
			},
		},
	}

	// Target addon with Pending status (not Ready)
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 2,
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.False(t, ruleStatuses[0].Matched)
	assert.Contains(t, ruleStatuses[0].Message, "Criterion not met")
	assert.Empty(t, activeSelectors)
}

func TestRuleEvaluator_ExternalSource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	// Create an external resource
	externalAddon := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addons.in-cloud.io/v1alpha1",
			"kind":       "Addon",
			"metadata": map[string]interface{}{
				"name": "cert-manager",
			},
			"status": map[string]interface{}{
				"observedGeneration": 1,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(externalAddon).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-certmanager-ready",
					Criteria: []addonsv1alpha1.Criterion{
						{
							Source: &addonsv1alpha1.CriterionSource{
								APIVersion: "addons.in-cloud.io/v1alpha1",
								Kind:       "Addon",
								Name:       "cert-manager",
							},
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "certificates",
						Priority:    20,
						MatchLabels: map[string]string{"feature": "certs"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_ExternalSourceNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-certmanager-ready",
					Criteria: []addonsv1alpha1.Criterion{
						{
							Source: &addonsv1alpha1.CriterionSource{
								APIVersion: "addons.in-cloud.io/v1alpha1",
								Kind:       "Addon",
								Name:       "non-existent",
							},
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "certificates",
						Priority:    20,
						MatchLabels: map[string]string{"feature": "certs"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.False(t, ruleStatuses[0].Matched)
	assert.Contains(t, ruleStatuses[0].Message, "not found")
	assert.Empty(t, activeSelectors)
}

func TestRuleEvaluator_MultipleCriteria(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "multi-criteria",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
						{
							JSONPath: "$.metadata.name",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`"test-phase"`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "full-match",
						Priority:    30,
						MatchLabels: map[string]string{"feature": "full"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_MultipleRules(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name:     "always-active",
					Criteria: nil,
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "base",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "base"},
					},
				},
				{
					Name: "when-ready",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "ready-only",
						Priority:    20,
						MatchLabels: map[string]string{"feature": "ready"},
					},
				},
				{
					Name: "when-failed",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`999`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "failed-only",
						Priority:    30,
						MatchLabels: map[string]string{"feature": "failed"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 3)

	// First rule: always active
	assert.True(t, ruleStatuses[0].Matched)
	// Second rule: Ready matches
	assert.True(t, ruleStatuses[1].Matched)
	// Third rule: Failed doesn't match
	assert.False(t, ruleStatuses[2].Matched)

	// 2 active selectors
	assert.Len(t, activeSelectors, 2)
	assert.Equal(t, "base", activeSelectors[0].Name)
	assert.Equal(t, "ready-only", activeSelectors[1].Name)
}

func TestRuleEvaluator_ExistsOperator(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-has-phase",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorExists,
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "has-phase",
						Priority:    10,
						MatchLabels: map[string]string{"has": "phase"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_NotExistsOperator(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-no-error",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.error",
							Operator: addonsv1alpha1.OperatorNotExists,
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "no-error",
						Priority:    10,
						MatchLabels: map[string]string{"has": "no-error"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
			// No error field
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_InOperator(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "when-phase-in-list",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorIn,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`[1, 2, 3]`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "active",
						Priority:    10,
						MatchLabels: map[string]string{"state": "active"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-phase",
		},
		Status: addonsv1alpha1.AddonStatus{
			ObservedGeneration: 1,
		},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.Len(t, activeSelectors, 1)
}

func boolPtr(b bool) *bool { return &b }

func TestRuleEvaluator_LatchOnFirstMatch_DefaultKeep(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "latch-rule",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
							// Keep is nil (default = true)
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "latched",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "latched"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Status:     addonsv1alpha1.AddonStatus{ObservedGeneration: 1},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched)
	assert.True(t, ruleStatuses[0].Latched, "Rule should be latched on first match with default keep")
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_LatchExplicitKeepTrue(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "explicit-keep",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
							Keep:     boolPtr(true),
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "kept",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "kept"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Status:     addonsv1alpha1.AddonStatus{ObservedGeneration: 1},
	}

	ruleStatuses, _, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.True(t, ruleStatuses[0].Latched)
}

func TestRuleEvaluator_NoLatchWhenKeepFalse(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "no-latch",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
							Keep:     boolPtr(false),
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "volatile",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "volatile"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Status:     addonsv1alpha1.AddonStatus{ObservedGeneration: 1},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.True(t, ruleStatuses[0].Matched)
	assert.False(t, ruleStatuses[0].Latched, "Rule should NOT latch when keep=false")
	assert.Len(t, activeSelectors, 1)
}

func TestRuleEvaluator_LatchMixedKeep(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	evaluator := NewRuleEvaluator(client)

	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "mixed-keep",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
							// keep=nil (true) — will latch
						},
						{
							JSONPath: "$.metadata.name",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`"test-phase"`)},
							Keep:     boolPtr(false), // re-evaluated every cycle
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "mixed",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "mixed"},
					},
				},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Status:     addonsv1alpha1.AddonStatus{ObservedGeneration: 1},
	}

	ruleStatuses, _, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.True(t, ruleStatuses[0].Matched)
	assert.True(t, ruleStatuses[0].Latched, "Rule SHOULD latch — has keepable criteria")
}

func TestRuleEvaluator_MixedKeep_KeepFalseFailsAfterLatch(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	evaluator := NewRuleEvaluator(client)

	// Previously latched rule with mixed keep criteria
	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "mixed-latched",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
							// keep=nil (true) — latched, won't be re-evaluated
						},
						{
							JSONPath: "$.metadata.name",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`"wrong-name"`)},
							Keep:     boolPtr(false), // re-evaluated — will fail
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "mixed",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "mixed"},
					},
				},
			},
		},
		Status: addonsv1alpha1.AddonPhaseStatus{
			RuleStatuses: []addonsv1alpha1.RuleStatus{
				{Name: "mixed-latched", Matched: true, Latched: true},
			},
		},
	}

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Status:     addonsv1alpha1.AddonStatus{ObservedGeneration: 999},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.False(t, ruleStatuses[0].Matched, "Rule should NOT match — keep=false criterion fails")
	assert.False(t, ruleStatuses[0].Latched, "Rule should lose latched status when not matched")
	assert.Empty(t, activeSelectors)
}

func TestRuleEvaluator_LatchedRuleSurvivesFailedCriteria(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	evaluator := NewRuleEvaluator(client)

	// Phase with pre-existing latched status (simulating a previous reconcile)
	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{
				{
					Name: "latched-rule",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "$.status.observedGeneration",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "persistent",
						Priority:    10,
						MatchLabels: map[string]string{"feature": "persistent"},
					},
				},
			},
		},
		Status: addonsv1alpha1.AddonPhaseStatus{
			RuleStatuses: []addonsv1alpha1.RuleStatus{
				{
					Name:    "latched-rule",
					Matched: true,
					Latched: true,
					Message: "Latched (keep)",
				},
			},
		},
	}

	// Addon where criteria would NOW fail (generation changed)
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "test-phase"},
		Status:     addonsv1alpha1.AddonStatus{ObservedGeneration: 999},
	}

	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(context.Background(), phase, addon)
	require.NoError(t, err)
	assert.Len(t, ruleStatuses, 1)
	assert.True(t, ruleStatuses[0].Matched, "Latched rule should stay matched")
	assert.True(t, ruleStatuses[0].Latched, "Latched status should persist")
	assert.Equal(t, "All conditions satisfied", ruleStatuses[0].Message)
	assert.Len(t, activeSelectors, 1, "Latched rule should still contribute selector")
}

func TestHasKeepableCriteria(t *testing.T) {
	tests := []struct {
		name     string
		rule     addonsv1alpha1.PhaseRule
		expected bool
	}{
		{
			name:     "no criteria = keepable",
			rule:     addonsv1alpha1.PhaseRule{Name: "empty"},
			expected: true,
		},
		{
			name: "all nil keep = has keepable",
			rule: addonsv1alpha1.PhaseRule{
				Name: "all-nil",
				Criteria: []addonsv1alpha1.Criterion{
					{JSONPath: "$.a", Operator: addonsv1alpha1.OperatorExists},
					{JSONPath: "$.b", Operator: addonsv1alpha1.OperatorExists},
				},
			},
			expected: true,
		},
		{
			name: "all true = has keepable",
			rule: addonsv1alpha1.PhaseRule{
				Name: "all-true",
				Criteria: []addonsv1alpha1.Criterion{
					{JSONPath: "$.a", Operator: addonsv1alpha1.OperatorExists, Keep: boolPtr(true)},
				},
			},
			expected: true,
		},
		{
			name: "mixed = has keepable (one nil + one false)",
			rule: addonsv1alpha1.PhaseRule{
				Name: "mixed",
				Criteria: []addonsv1alpha1.Criterion{
					{JSONPath: "$.a", Operator: addonsv1alpha1.OperatorExists},
					{JSONPath: "$.b", Operator: addonsv1alpha1.OperatorExists, Keep: boolPtr(false)},
				},
			},
			expected: true,
		},
		{
			name: "all false = no keepable criteria",
			rule: addonsv1alpha1.PhaseRule{
				Name: "all-false",
				Criteria: []addonsv1alpha1.Criterion{
					{JSONPath: "$.a", Operator: addonsv1alpha1.OperatorExists, Keep: boolPtr(false)},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasKeepableCriteria(tt.rule))
		})
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name     string
		actual   interface{}
		operator addonsv1alpha1.CriterionOperator
		expected *apiextensionsv1.JSON
		found    bool
		matched  bool
	}{
		{
			name:     "equal strings",
			actual:   "Ready",
			operator: addonsv1alpha1.OperatorEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "not equal",
			actual:   "Pending",
			operator: addonsv1alpha1.OperatorNotEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "exists when present",
			actual:   "value",
			operator: addonsv1alpha1.OperatorExists,
			found:    true,
			matched:  true,
		},
		{
			name:     "not exists when not found",
			actual:   nil,
			operator: addonsv1alpha1.OperatorNotExists,
			found:    false,
			matched:  true,
		},
		{
			name:     "in list",
			actual:   "Ready",
			operator: addonsv1alpha1.OperatorIn,
			expected: &apiextensionsv1.JSON{Raw: []byte(`["Ready", "Pending"]`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "not in list",
			actual:   "Ready",
			operator: addonsv1alpha1.OperatorNotIn,
			expected: &apiextensionsv1.JSON{Raw: []byte(`["Failed", "Pending"]`)},
			found:    true,
			matched:  true,
		},
		// GreaterThan tests
		{
			name:     "greater than - true",
			actual:   float64(10),
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "greater than - false when equal",
			actual:   float64(5),
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
		{
			name:     "greater than - false when less",
			actual:   float64(3),
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
		// GreaterOrEqual tests
		{
			name:     "greater or equal - true when greater",
			actual:   float64(10),
			operator: addonsv1alpha1.OperatorGreaterOrEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "greater or equal - true when equal",
			actual:   float64(5),
			operator: addonsv1alpha1.OperatorGreaterOrEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "greater or equal - false when less",
			actual:   float64(3),
			operator: addonsv1alpha1.OperatorGreaterOrEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
		// LessThan tests
		{
			name:     "less than - true",
			actual:   float64(3),
			operator: addonsv1alpha1.OperatorLessThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "less than - false when equal",
			actual:   float64(5),
			operator: addonsv1alpha1.OperatorLessThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
		{
			name:     "less than - false when greater",
			actual:   float64(10),
			operator: addonsv1alpha1.OperatorLessThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
		// LessOrEqual tests
		{
			name:     "less or equal - true when less",
			actual:   float64(3),
			operator: addonsv1alpha1.OperatorLessOrEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "less or equal - true when equal",
			actual:   float64(5),
			operator: addonsv1alpha1.OperatorLessOrEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "less or equal - false when greater",
			actual:   float64(10),
			operator: addonsv1alpha1.OperatorLessOrEqual,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
		// Matches tests
		{
			name:     "matches - true with simple pattern",
			actual:   "prod-cluster-1",
			operator: addonsv1alpha1.OperatorMatches,
			expected: &apiextensionsv1.JSON{Raw: []byte(`"^prod-.*"`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "matches - false when no match",
			actual:   "dev-cluster-1",
			operator: addonsv1alpha1.OperatorMatches,
			expected: &apiextensionsv1.JSON{Raw: []byte(`"^prod-.*"`)},
			found:    true,
			matched:  false,
		},
		{
			name:     "matches - true with complex pattern",
			actual:   "app-v1.2.3",
			operator: addonsv1alpha1.OperatorMatches,
			expected: &apiextensionsv1.JSON{Raw: []byte(`"^app-v[0-9]+\\.[0-9]+\\.[0-9]+$"`)},
			found:    true,
			matched:  true,
		},
		// Numeric with int types
		{
			name:     "greater than - int actual",
			actual:   10,
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		{
			name:     "greater than - int64 actual",
			actual:   int64(10),
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		// Numeric comparison with string numbers
		{
			name:     "greater than - string number actual",
			actual:   "10",
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  true,
		},
		// Type mismatch - non-numeric with numeric operator
		{
			name:     "greater than - non-numeric returns false",
			actual:   "not-a-number",
			operator: addonsv1alpha1.OperatorGreaterThan,
			expected: &apiextensionsv1.JSON{Raw: []byte(`5`)},
			found:    true,
			matched:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := compareValues(tt.actual, tt.operator, tt.expected, tt.found)
			require.NoError(t, err)
			assert.Equal(t, tt.matched, matched)
		})
	}
}
