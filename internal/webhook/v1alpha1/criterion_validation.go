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

package v1alpha1

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/pkg/criteria/jsonpath"
)

// operatorsRequiringValue lists operators that require the Value field to be set.
var operatorsRequiringValue = map[addonsv1alpha1.CriterionOperator]bool{
	addonsv1alpha1.OperatorEqual:          true,
	addonsv1alpha1.OperatorNotEqual:       true,
	addonsv1alpha1.OperatorIn:             true,
	addonsv1alpha1.OperatorNotIn:          true,
	addonsv1alpha1.OperatorGreaterThan:    true,
	addonsv1alpha1.OperatorGreaterOrEqual: true,
	addonsv1alpha1.OperatorLessThan:       true,
	addonsv1alpha1.OperatorLessOrEqual:    true,
	addonsv1alpha1.OperatorMatches:        true,
}

// operatorsForbiddingValue lists operators that must not have the Value field set.
var operatorsForbiddingValue = map[addonsv1alpha1.CriterionOperator]bool{
	addonsv1alpha1.OperatorExists:    true,
	addonsv1alpha1.OperatorNotExists: true,
}

// validateCriterion validates a single Criterion's operator+value consistency
// and source field constraints.
//
//nolint:gocyclo // validation of multiple operator constraints
func validateCriterion(criterion addonsv1alpha1.Criterion, path string) error {
	if operatorsRequiringValue[criterion.Operator] && criterion.Value == nil {
		return fmt.Errorf("%s: operator %s requires a value", path, criterion.Operator)
	}
	if operatorsForbiddingValue[criterion.Operator] && criterion.Value != nil {
		return fmt.Errorf("%s: operator %s must not have a value", path, criterion.Operator)
	}

	if criterion.JSONPath != "" {
		if err := jsonpath.Parse(criterion.JSONPath); err != nil {
			return fmt.Errorf("%s: invalid jsonPath %q: %w", path, criterion.JSONPath, err)
		}
	}

	if criterion.Operator == addonsv1alpha1.OperatorMatches && criterion.Value != nil {
		pattern, err := extractStringValue(criterion.Value)
		if err == nil {
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("%s: invalid regex pattern for Matches operator: %w", path, err)
			}
		}
	}

	if criterion.Source != nil {
		if err := validateCriterionSource(criterion.Source, path); err != nil {
			return err
		}
	}

	return nil
}

// validateCriterionSource validates CriterionSource field constraints.
// Name and LabelSelector are mutually exclusive for target identification.
func validateCriterionSource(source *addonsv1alpha1.CriterionSource, path string) error {
	if source.Name != "" && source.LabelSelector != nil {
		return fmt.Errorf("%s: source name and labelSelector are mutually exclusive", path)
	}
	if source.Name == "" && source.LabelSelector == nil {
		return fmt.Errorf("%s: source must specify either name or labelSelector", path)
	}

	return nil
}

// validateCriteria validates a slice of criteria.
func validateCriteria(criteria []addonsv1alpha1.Criterion, pathPrefix string) error {
	for i, criterion := range criteria {
		path := fmt.Sprintf("%s[%d]", pathPrefix, i)
		if err := validateCriterion(criterion, path); err != nil {
			return err
		}
	}

	return nil
}

// validateKeepImmutability checks that the effective keep value of existing criteria
// has not changed between old and new specs. nil and *true are equivalent (both mean keep=true).
func validateKeepImmutability(oldPhase, newPhase *addonsv1alpha1.AddonPhase) error {
	oldRules := make(map[string]addonsv1alpha1.PhaseRule, len(oldPhase.Spec.Rules))
	for _, r := range oldPhase.Spec.Rules {
		oldRules[r.Name] = r
	}

	for _, newRule := range newPhase.Spec.Rules {
		oldRule, exists := oldRules[newRule.Name]
		if !exists {
			continue
		}

		minLen := min(len(newRule.Criteria), len(oldRule.Criteria))

		for i := range minLen {
			oldEffective := effectiveKeep(oldRule.Criteria[i].Keep)
			newEffective := effectiveKeep(newRule.Criteria[i].Keep)
			if oldEffective != newEffective {
				return fmt.Errorf("spec.rules[%s].criteria[%d]: keep value is immutable (was %v, got %v)",
					newRule.Name, i, oldEffective, newEffective)
			}
		}
	}

	return nil
}

func effectiveKeep(keep *bool) bool {
	if keep == nil {
		return true
	}

	return *keep
}

// extractStringValue extracts a string from apiextensionsv1.JSON.
func extractStringValue(j *apiextensionsv1.JSON) (string, error) {
	if j == nil || len(j.Raw) == 0 {
		return "", errors.New("empty value")
	}
	var s string
	if err := json.Unmarshal(j.Raw, &s); err != nil {
		return "", err
	}

	return s, nil
}
