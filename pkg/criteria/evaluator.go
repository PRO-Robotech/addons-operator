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
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"addons-operator/pkg/criteria/jsonpath"
)

// Evaluator evaluates criteria against objects.
type Evaluator struct{}

// NewEvaluator creates a new Evaluator instance.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// EvaluationResult contains the result of a criterion evaluation.
type EvaluationResult struct {
	Satisfied bool
	Reason    string
	Error     error
}

// Evaluate evaluates a single criterion against an object.
// Returns (satisfied, reason, error).
func (e *Evaluator) Evaluate(obj any, path string, operator Operator, expected *apiextensionsv1.JSON) EvaluationResult {
	// Extract value from object using RFC 9535 JSONPath
	actual, found, err := jsonpath.ExtractString(obj, path)
	if err != nil {
		return EvaluationResult{
			Satisfied: false,
			Reason:    fmt.Sprintf("invalid path: %s", err),
			Error:     err,
		}
	}

	// Handle existence operators first
	switch operator {
	case OperatorExists:
		return EvaluationResult{
			Satisfied: EvalExists(found),
			Reason:    reasonForExists(path, found),
		}
	case OperatorNotExists:
		return EvaluationResult{
			Satisfied: EvalNotExists(found),
			Reason:    reasonForNotExists(path, found),
		}
	}

	// For other operators, if path not found, criterion is not satisfied
	if !found {
		return EvaluationResult{
			Satisfied: false,
			Reason:    fmt.Sprintf("path %s not found", path),
		}
	}

	// Evaluate based on operator
	return e.evaluateOperator(actual, operator, expected, path)
}

// EvaluateAll evaluates multiple criteria against an object.
// Returns true only if ALL criteria are satisfied.
func (e *Evaluator) EvaluateAll(obj any, criteria []CriterionInput) (bool, []EvaluationResult) {
	results := make([]EvaluationResult, len(criteria))
	allSatisfied := true

	for i, c := range criteria {
		results[i] = e.Evaluate(obj, c.Path, c.Operator, c.Expected)
		if !results[i].Satisfied {
			allSatisfied = false
		}
	}

	return allSatisfied, results
}

// CriterionInput represents input for a criterion evaluation.
type CriterionInput struct {
	Path     string
	Operator Operator
	Expected *apiextensionsv1.JSON
}

// evaluateOperator evaluates the appropriate operator.
func (e *Evaluator) evaluateOperator(
	actual any, operator Operator, expected *apiextensionsv1.JSON, path string,
) EvaluationResult {
	var satisfied bool
	var err error

	switch operator {
	case OperatorEqual:
		satisfied, err = EvalEqual(actual, expected)
	case OperatorNotEqual:
		satisfied, err = EvalNotEqual(actual, expected)
	case OperatorIn:
		satisfied, err = EvalIn(actual, expected)
	case OperatorNotIn:
		satisfied, err = EvalNotIn(actual, expected)
	case OperatorGreaterThan:
		satisfied, err = EvalGreaterThan(actual, expected)
	case OperatorGreaterOrEqual:
		satisfied, err = EvalGreaterOrEqual(actual, expected)
	case OperatorLessThan:
		satisfied, err = EvalLessThan(actual, expected)
	case OperatorLessOrEqual:
		satisfied, err = EvalLessOrEqual(actual, expected)
	case OperatorMatches:
		satisfied, err = EvalMatches(actual, expected)
	default:
		return EvaluationResult{
			Satisfied: false,
			Reason:    fmt.Sprintf("unknown operator: %s", operator),
			Error:     fmt.Errorf("unknown operator: %s", operator),
		}
	}

	if err != nil {
		return EvaluationResult{
			Satisfied: false,
			Reason:    fmt.Sprintf("evaluation error: %s", err),
			Error:     err,
		}
	}

	return EvaluationResult{
		Satisfied: satisfied,
		Reason:    reasonForComparison(path, operator, satisfied),
	}
}

func reasonForExists(path string, found bool) string {
	if found {
		return fmt.Sprintf("path %s exists", path)
	}

	return fmt.Sprintf("path %s does not exist", path)
}

func reasonForNotExists(path string, found bool) string {
	if !found {
		return fmt.Sprintf("path %s does not exist (as expected)", path)
	}

	return fmt.Sprintf("path %s exists (but should not)", path)
}

func reasonForComparison(path string, operator Operator, satisfied bool) string {
	if satisfied {
		return fmt.Sprintf("path %s %s comparison satisfied", path, operator)
	}

	return fmt.Sprintf("path %s %s comparison not satisfied", path, operator)
}
