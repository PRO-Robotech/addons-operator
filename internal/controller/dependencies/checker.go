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
	"encoding/json"
	"fmt"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/pkg/criteria"
	"addons-operator/pkg/criteria/jsonpath"
)

// DependencyChecker verifies initDependencies conditions are satisfied.
// It implements a one-time gate: once the Application is created,
// dependencies are no longer checked.
type DependencyChecker struct {
	client client.Client
}

// NewDependencyChecker creates a new DependencyChecker.
func NewDependencyChecker(c client.Client) *DependencyChecker {
	return &DependencyChecker{
		client: c,
	}
}

// CheckResult represents the result of dependency checking.
type CheckResult struct {
	Satisfied bool
	Reason    string
}

func (c *DependencyChecker) CheckDependencies(ctx context.Context, addon *addonsv1alpha1.Addon, appNamespace string) (CheckResult, error) {
	if len(addon.Spec.InitDependencies) == 0 {
		return CheckResult{Satisfied: true}, nil
	}

	// One-time gate: if Application already exists, dependencies were satisfied
	app := &argocdv1alpha1.Application{}
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      addon.Name,
		Namespace: appNamespace,
	}, app)
	if err == nil {
		return CheckResult{Satisfied: true}, nil
	}
	if !apierrors.IsNotFound(err) {
		return CheckResult{}, fmt.Errorf("get Application: %w", err)
	}

	for _, dep := range addon.Spec.InitDependencies {
		result, err := c.checkDependency(ctx, dep)
		if err != nil {
			return CheckResult{}, err
		}
		if !result.Satisfied {
			return result, nil
		}
	}

	return CheckResult{Satisfied: true}, nil
}

func (c *DependencyChecker) checkDependency(ctx context.Context, dep addonsv1alpha1.Dependency) (CheckResult, error) {
	depAddon := &addonsv1alpha1.Addon{}
	err := c.client.Get(ctx, types.NamespacedName{Name: dep.Name}, depAddon)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return CheckResult{
				Satisfied: false,
				Reason:    fmt.Sprintf("Addon %s not found", dep.Name),
			}, nil
		}
		return CheckResult{}, fmt.Errorf("get Addon %s: %w", dep.Name, err)
	}

	for _, criterion := range dep.Criteria {
		result, err := c.evaluateCriterion(ctx, depAddon, criterion)
		if err != nil {
			return CheckResult{}, fmt.Errorf("evaluate criterion for %s: %w", dep.Name, err)
		}
		if !result.Satisfied {
			return result, nil
		}
	}

	return CheckResult{Satisfied: true}, nil
}

func (c *DependencyChecker) evaluateCriterion(ctx context.Context, depAddon *addonsv1alpha1.Addon, criterion addonsv1alpha1.Criterion) (CheckResult, error) {
	var obj any

	if criterion.Source != nil {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.FromAPIVersionAndKind(
			criterion.Source.APIVersion,
			criterion.Source.Kind,
		))

		err := c.client.Get(ctx, types.NamespacedName{
			Name:      criterion.Source.Name,
			Namespace: criterion.Source.Namespace,
		}, u)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return CheckResult{
					Satisfied: false,
					Reason:    fmt.Sprintf("Resource %s/%s not found", criterion.Source.Kind, criterion.Source.Name),
				}, nil
			}
			return CheckResult{}, fmt.Errorf("get resource %s/%s: %w", criterion.Source.Kind, criterion.Source.Name, err)
		}
		obj = u.Object
	} else {
		data, err := json.Marshal(depAddon)
		if err != nil {
			return CheckResult{}, fmt.Errorf("marshal addon: %w", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return CheckResult{}, fmt.Errorf("unmarshal addon: %w", err)
		}
		obj = m
	}

	actualValue, found, err := jsonpath.ExtractString(obj, criterion.JSONPath)
	if err != nil {
		return CheckResult{}, fmt.Errorf("extract JSONPath %s: %w", criterion.JSONPath, err)
	}
	if !found {
		return CheckResult{
			Satisfied: false,
			Reason:    fmt.Sprintf("Path %s not found in %s", criterion.JSONPath, depAddon.Name),
		}, nil
	}

	matched, err := compareValues(actualValue, criterion.Operator, criterion.Value)
	if err != nil {
		return CheckResult{}, fmt.Errorf("compare values: %w", err)
	}

	if !matched {
		expectedValue := "<nil>"
		if criterion.Value != nil {
			expectedValue = string(criterion.Value.Raw)
		}
		return CheckResult{
			Satisfied: false,
			Reason: fmt.Sprintf("Waiting for %s: %s %s %s (actual: %v)",
				depAddon.Name, criterion.JSONPath, criterion.Operator, expectedValue, actualValue),
		}, nil
	}

	return CheckResult{Satisfied: true}, nil
}

// compareValues delegates to pkg/criteria for value comparison.
func compareValues(actual any, operator addonsv1alpha1.CriterionOperator, expected *apiextensionsv1.JSON) (bool, error) {
	op := criteria.Operator(operator)
	switch op {
	case criteria.OperatorEqual:
		return criteria.EvalEqual(actual, expected)
	case criteria.OperatorNotEqual:
		return criteria.EvalNotEqual(actual, expected)
	case criteria.OperatorExists:
		return criteria.EvalExists(actual != nil), nil
	case criteria.OperatorNotExists:
		return criteria.EvalNotExists(actual != nil), nil
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
