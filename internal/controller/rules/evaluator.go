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

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/pkg/criteria"
	"addons-operator/pkg/criteria/jsonpath"
)

type RuleEvaluator struct {
	client client.Client
	// cacheBackedGK is the set of GroupKinds that already have a shared informer in
	// the manager cache (the types the controllers watch). Reads of these are served
	// from cache; any other kind stays on the uncached unstructured path so we never
	// start a new cluster-wide informer (e.g. for ConfigMaps or Secrets).
	cacheBackedGK map[schema.GroupKind]bool
}

func NewRuleEvaluator(c client.Client) *RuleEvaluator {
	return &RuleEvaluator{
		client:        c,
		cacheBackedGK: buildCacheBackedGK(c.Scheme()),
	}
}

// buildCacheBackedGK derives, from the scheme, the GroupKinds whose informers the
// manager already runs (Addon, AddonPhase, AddonValue, Application). Types absent
// from the scheme are simply skipped, so unit tests with a minimal scheme degrade
// gracefully to the unstructured path.
func buildCacheBackedGK(s *runtime.Scheme) map[schema.GroupKind]bool {
	gks := make(map[schema.GroupKind]bool)
	if s == nil {
		return gks
	}

	for _, obj := range []client.Object{
		&addonsv1alpha1.Addon{},
		&addonsv1alpha1.AddonPhase{},
		&addonsv1alpha1.AddonValue{},
		&argocdv1alpha1.Application{},
	} {
		kinds, _, err := s.ObjectKinds(obj)
		if err != nil {
			continue
		}
		for _, gvk := range kinds {
			gks[gvk.GroupKind()] = true
		}
	}

	return gks
}

// evalContext carries the target addon plus per-EvaluateRules read caches. A single
// AddonPhase reconcile evaluates many criteria that often reference the same source
// object and always reference the same target addon; without memoization each
// criterion re-fetched the source (uncached API GET) and re-marshalled the addon.
// The context lives on the stack for one EvaluateRules call, so it is safe to share
// across concurrent reconcile workers.
type evalContext struct {
	targetAddon *addonsv1alpha1.Addon

	targetMap  map[string]any
	targetErr  error
	targetDone bool

	sources map[string]sourceEntry
}

type sourceEntry struct {
	obj   map[string]any
	found bool
}

func newEvalContext(addon *addonsv1alpha1.Addon) *evalContext {
	return &evalContext{targetAddon: addon, sources: make(map[string]sourceEntry)}
}

// addonMap converts the target addon to a map exactly once per EvaluateRules call,
// preserving the original json round-trip semantics for JSONPath evaluation.
func (ec *evalContext) addonMap() (map[string]any, error) {
	if ec.targetDone {
		return ec.targetMap, ec.targetErr
	}
	ec.targetDone = true

	data, err := json.Marshal(ec.targetAddon)
	if err != nil {
		ec.targetErr = fmt.Errorf("marshal addon: %w", err)
		return nil, ec.targetErr
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		ec.targetErr = fmt.Errorf("unmarshal addon: %w", err)
		return nil, ec.targetErr
	}
	ec.targetMap = m

	return ec.targetMap, nil
}

// resolveSource fetches the criterion source object at most once per evaluation,
// caching both hits and not-found results.
func (e *RuleEvaluator) resolveSource(
	ctx context.Context,
	src *addonsv1alpha1.CriterionSource,
	ec *evalContext,
) (map[string]any, bool, error) {
	key := src.APIVersion + "|" + src.Kind + "|" + src.Namespace + "|" + src.Name
	if entry, ok := ec.sources[key]; ok {
		return entry.obj, entry.found, nil
	}

	obj, err := e.getSource(ctx, src)
	if err != nil {
		if apierrors.IsNotFound(err) {
			ec.sources[key] = sourceEntry{found: false}
			return nil, false, nil
		}
		return nil, false, err
	}

	ec.sources[key] = sourceEntry{obj: obj, found: true}

	return obj, true, nil
}

// getSource reads a source object. For kinds already backed by a cache informer it
// reads the typed object (a cache hit with no API round-trip — this removes the
// io.ReadAll churn); for every other kind it falls back to an uncached unstructured
// GET so no new cluster-wide informer is started.
func (e *RuleEvaluator) getSource(
	ctx context.Context,
	src *addonsv1alpha1.CriterionSource,
) (map[string]any, error) {
	gvk := schema.FromAPIVersionAndKind(src.APIVersion, src.Kind)
	key := types.NamespacedName{Name: src.Name, Namespace: src.Namespace}

	if e.cacheBackedGK[gvk.GroupKind()] {
		if obj, err := e.client.Scheme().New(gvk); err == nil {
			if cObj, ok := obj.(client.Object); ok {
				if getErr := e.client.Get(ctx, key, cObj); getErr != nil {
					return nil, getErr
				}
				m, convErr := runtime.DefaultUnstructuredConverter.ToUnstructured(cObj)
				if convErr != nil {
					return nil, fmt.Errorf("convert %s/%s to map: %w", src.Kind, src.Name, convErr)
				}
				return m, nil
			}
		}
		// Scheme could not build a typed object: fall through to unstructured.
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	if err := e.client.Get(ctx, key, u); err != nil {
		return nil, err
	}

	return u.Object, nil
}

func (e *RuleEvaluator) EvaluateRules(
	ctx context.Context,
	phase *addonsv1alpha1.AddonPhase,
	targetAddon *addonsv1alpha1.Addon,
) ([]addonsv1alpha1.RuleStatus, []addonsv1alpha1.ValuesSelector, error) {
	previousStatuses := buildPreviousStatusMap(phase)
	ruleStatuses := make([]addonsv1alpha1.RuleStatus, 0, len(phase.Spec.Rules))
	activeSelectors := make([]addonsv1alpha1.ValuesSelector, 0, len(phase.Spec.Rules))

	ec := newEvalContext(targetAddon)

	for _, rule := range phase.Spec.Rules {
		prev := previousStatuses[rule.Name]

		matched, message, err := e.evaluateRule(ctx, rule, ec, prev.Latched)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluate rule %s: %w", rule.Name, err)
		}

		latched := matched && hasKeepableCriteria(rule)

		lastEvaluated := metav1.Now()
		if prev.Name != "" && prev.Matched == matched &&
			prev.Latched == latched &&
			prev.Message == message {
			lastEvaluated = prev.LastEvaluated
		}

		ruleStatuses = append(ruleStatuses, addonsv1alpha1.RuleStatus{
			Name:          rule.Name,
			Matched:       matched,
			Latched:       latched,
			Message:       message,
			LastEvaluated: lastEvaluated,
			Deployed:      prev.Deployed,
		})

		if matched {
			activeSelectors = append(activeSelectors, rule.Selector)
		}
	}

	return ruleStatuses, activeSelectors, nil
}

func buildPreviousStatusMap(phase *addonsv1alpha1.AddonPhase) map[string]addonsv1alpha1.RuleStatus {
	m := make(map[string]addonsv1alpha1.RuleStatus, len(phase.Status.RuleStatuses))
	for _, rs := range phase.Status.RuleStatuses {
		m[rs.Name] = rs
	}

	return m
}

func hasKeepableCriteria(rule addonsv1alpha1.PhaseRule) bool {
	for _, c := range rule.Criteria {
		if c.Keep == nil || *c.Keep {
			return true
		}
	}

	return len(rule.Criteria) == 0
}

func isKeepCriterion(c addonsv1alpha1.Criterion) bool {
	return c.Keep == nil || *c.Keep
}

func (e *RuleEvaluator) evaluateRule(
	ctx context.Context,
	rule addonsv1alpha1.PhaseRule,
	ec *evalContext,
	previouslyLatched bool,
) (bool, string, error) {
	if len(rule.Criteria) == 0 {
		return true, "No conditions", nil
	}

	for i, criterion := range rule.Criteria {
		if previouslyLatched && isKeepCriterion(criterion) {
			continue
		}

		matched, reason, err := e.evaluateCriterion(ctx, criterion, ec)
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
	ec *evalContext,
) (bool, string, error) {
	var obj any

	if criterion.Source != nil {
		resolved, found, err := e.resolveSource(ctx, criterion.Source, ec)
		if err != nil {
			return false, "", fmt.Errorf("get resource %s/%s: %w", criterion.Source.Kind, criterion.Source.Name, err)
		}
		if !found {
			return false, fmt.Sprintf("Resource %s/%s not found", criterion.Source.Kind, criterion.Source.Name), nil
		}
		obj = resolved
	} else {
		m, err := ec.addonMap()
		if err != nil {
			return false, "", err
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
