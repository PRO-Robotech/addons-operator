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
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/pkg/criteria"
	"addons-operator/pkg/criteria/jsonpath"
)

type RuleEvaluator struct {
	client client.Client
}

func NewRuleEvaluator(c client.Client) *RuleEvaluator {
	return &RuleEvaluator{client: c}
}

func (e *RuleEvaluator) EvaluateRules(
	ctx context.Context,
	phase *addonsv1alpha1.AddonPhase,
	targetAddon *addonsv1alpha1.Addon,
) ([]addonsv1alpha1.RuleStatus, []addonsv1alpha1.ValuesSelector, error) {

	ruleStatuses := make([]addonsv1alpha1.RuleStatus, 0, len(phase.Spec.Rules))
	activeSelectors := make([]addonsv1alpha1.ValuesSelector, 0, len(phase.Spec.Rules))

	for _, rule := range phase.Spec.Rules {
		matched, message, err := e.evaluateRule(ctx, rule, targetAddon)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluate rule %s: %w", rule.Name, err)
		}

		ruleStatuses = append(ruleStatuses, addonsv1alpha1.RuleStatus{
			Name:          rule.Name,
			Matched:       matched,
			Message:       message,
			LastEvaluated: metav1.Now(),
		})

		if matched {
			activeSelectors = append(activeSelectors, rule.Selector)
		}
	}

	return ruleStatuses, activeSelectors, nil
}

func (e *RuleEvaluator) evaluateRule(
	ctx context.Context,
	rule addonsv1alpha1.PhaseRule,
	targetAddon *addonsv1alpha1.Addon,
) (bool, string, error) {
	if len(rule.Criteria) == 0 {
		return true, "No conditions", nil
	}

	for i, criterion := range rule.Criteria {
		matched, reason, err := e.evaluateCriterion(ctx, criterion, targetAddon)
		if err != nil {
			return false, "", fmt.Errorf("criterion %d: %w", i, err)
		}
		if !matched {
			return false, reason, nil
		}
	}

	return true, "All conditions satisfied", nil
}

func (e *RuleEvaluator) evaluateCriterion(
	ctx context.Context,
	criterion addonsv1alpha1.Criterion,
	targetAddon *addonsv1alpha1.Addon,
) (bool, string, error) {
	var obj any

	if criterion.Source != nil {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.FromAPIVersionAndKind(
			criterion.Source.APIVersion,
			criterion.Source.Kind,
		))

		err := e.client.Get(ctx, types.NamespacedName{
			Name:      criterion.Source.Name,
			Namespace: criterion.Source.Namespace,
		}, u)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, fmt.Sprintf("Resource %s/%s not found", criterion.Source.Kind, criterion.Source.Name), nil
			}
			return false, "", fmt.Errorf("get resource %s/%s: %w", criterion.Source.Kind, criterion.Source.Name, err)
		}
		obj = u.Object
	} else {
		data, err := json.Marshal(targetAddon)
		if err != nil {
			return false, "", fmt.Errorf("marshal addon: %w", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return false, "", fmt.Errorf("unmarshal addon: %w", err)
		}
		obj = m
	}

	actualValue, found, err := jsonpath.ExtractString(obj, criterion.JSONPath)
	if err != nil {
		return false, "", fmt.Errorf("extract JSONPath %s: %w", criterion.JSONPath, err)
	}

	if !found {
		if criterion.Operator == addonsv1alpha1.OperatorNotExists {
			return true, "", nil
		}

		if criterion.Operator == addonsv1alpha1.OperatorExists {
			return false, fmt.Sprintf("Path %s does not exist", criterion.JSONPath), nil
		}
		return false, fmt.Sprintf("Path %s not found", criterion.JSONPath), nil
	}

	matched, err := compareValues(actualValue, criterion.Operator, criterion.Value, found)
	if err != nil {
		return false, "", fmt.Errorf("compare values: %w", err)
	}

	if !matched {
		expectedValue := "<nil>"
		if criterion.Value != nil {
			expectedValue = string(criterion.Value.Raw)
		}
		return false, fmt.Sprintf("Criterion not met: %s %s %s (actual: %v)",
			criterion.JSONPath, criterion.Operator, expectedValue, actualValue), nil
	}

	return true, "", nil
}

// compareValues delegates to pkg/criteria for value comparison.
func compareValues(actual any, operator addonsv1alpha1.CriterionOperator, expected *apiextensionsv1.JSON, found bool) (bool, error) {
	op := criteria.Operator(operator)
	switch op {
	case criteria.OperatorEqual:
		return criteria.EvalEqual(actual, expected)
	case criteria.OperatorNotEqual:
		return criteria.EvalNotEqual(actual, expected)
	case criteria.OperatorExists:
		return criteria.EvalExists(found), nil
	case criteria.OperatorNotExists:
		return criteria.EvalNotExists(found), nil
	case criteria.OperatorIn:
		return criteria.EvalIn(actual, expected)
	case criteria.OperatorNotIn:
		return criteria.EvalNotIn(actual, expected)
	case criteria.OperatorGreaterThan:
		return criteria.EvalGreaterThan(actual, expected)
	case criteria.OperatorGreaterOrEqual:
		return criteria.EvalGreaterOrEqual(actual, expected)
	case criteria.OperatorLessThan:
		return criteria.EvalLessThan(actual, expected)
	case criteria.OperatorLessOrEqual:
		return criteria.EvalLessOrEqual(actual, expected)
	case criteria.OperatorMatches:
		return criteria.EvalMatches(actual, expected)
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}
