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
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// Operator represents a comparison operator type.
type Operator string

const (
	// OperatorEqual checks if values are equal.
	OperatorEqual Operator = "Equal"
	// OperatorNotEqual checks if values are not equal.
	OperatorNotEqual Operator = "NotEqual"
	// OperatorIn checks if value is in a list.
	OperatorIn Operator = "In"
	// OperatorNotIn checks if value is not in a list.
	OperatorNotIn Operator = "NotIn"
	// OperatorGreaterThan checks if value is greater than.
	OperatorGreaterThan Operator = "GreaterThan"
	// OperatorGreaterOrEqual checks if value is greater than or equal.
	OperatorGreaterOrEqual Operator = "GreaterOrEqual"
	// OperatorLessThan checks if value is less than.
	OperatorLessThan Operator = "LessThan"
	// OperatorLessOrEqual checks if value is less than or equal.
	OperatorLessOrEqual Operator = "LessOrEqual"
	// OperatorExists checks if path exists in the object.
	OperatorExists Operator = "Exists"
	// OperatorNotExists checks if path does not exist.
	OperatorNotExists Operator = "NotExists"
	// OperatorMatches checks if value matches a regex pattern.
	OperatorMatches Operator = "Matches"
)

// EvalEqual compares two values for equality.
func EvalEqual(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	if expected == nil || len(expected.Raw) == 0 {
		return actual == nil, nil
	}

	expectedValue, err := parseJSON(expected)
	if err != nil {
		return false, err
	}

	return deepEqual(actual, expectedValue), nil
}

// EvalNotEqual compares two values for inequality.
func EvalNotEqual(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	eq, err := EvalEqual(actual, expected)
	return !eq, err
}

// EvalIn checks if actual value is in the expected list.
func EvalIn(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	if expected == nil || len(expected.Raw) == 0 {
		return false, nil
	}

	var list []any
	if err := json.Unmarshal(expected.Raw, &list); err != nil {
		return false, fmt.Errorf("expected value must be an array for In operator: %w", err)
	}

	for _, item := range list {
		if deepEqual(actual, item) {
			return true, nil
		}
	}
	return false, nil
}

// EvalNotIn checks if actual value is not in the expected list.
func EvalNotIn(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	in, err := EvalIn(actual, expected)
	return !in, err
}

// EvalGreaterThan checks if actual > expected (numeric).
func EvalGreaterThan(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	a, ok := toFloat64(actual)
	if !ok {
		return false, nil // type mismatch = false, not error
	}
	e, err := expectedToFloat64(expected)
	if err != nil {
		return false, err
	}
	return a > e, nil
}

// EvalGreaterOrEqual checks if actual >= expected (numeric).
func EvalGreaterOrEqual(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	a, ok := toFloat64(actual)
	if !ok {
		return false, nil
	}
	e, err := expectedToFloat64(expected)
	if err != nil {
		return false, err
	}
	return a >= e, nil
}

// EvalLessThan checks if actual < expected (numeric).
func EvalLessThan(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	a, ok := toFloat64(actual)
	if !ok {
		return false, nil
	}
	e, err := expectedToFloat64(expected)
	if err != nil {
		return false, err
	}
	return a < e, nil
}

// EvalLessOrEqual checks if actual <= expected (numeric).
func EvalLessOrEqual(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	a, ok := toFloat64(actual)
	if !ok {
		return false, nil
	}
	e, err := expectedToFloat64(expected)
	if err != nil {
		return false, err
	}
	return a <= e, nil
}

// EvalExists checks if the path exists (value was found).
func EvalExists(found bool) bool {
	return found
}

// EvalNotExists checks if the path does not exist.
func EvalNotExists(found bool) bool {
	return !found
}

// EvalMatches checks if actual matches the expected regex pattern.
func EvalMatches(actual any, expected *apiextensionsv1.JSON) (bool, error) {
	s, ok := actual.(string)
	if !ok {
		return false, nil // type mismatch = false
	}

	pattern, err := parseStringValue(expected)
	if err != nil {
		return false, err
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return re.MatchString(s), nil
}

// Helper functions

func parseJSON(j *apiextensionsv1.JSON) (any, error) {
	if j == nil || len(j.Raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(j.Raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

//nolint:unparam // error return is part of API contract for future extensibility
func parseStringValue(j *apiextensionsv1.JSON) (string, error) {
	if j == nil || len(j.Raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(j.Raw, &s); err != nil {
		// Try as raw string without quotes
		return strings.Trim(string(j.Raw), "\""), nil
	}
	return s, nil
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func expectedToFloat64(expected *apiextensionsv1.JSON) (float64, error) {
	if expected == nil || len(expected.Raw) == 0 {
		return 0, fmt.Errorf("expected value is required for numeric comparison")
	}

	var v any
	if err := json.Unmarshal(expected.Raw, &v); err != nil {
		return 0, fmt.Errorf("failed to parse expected value: %w", err)
	}

	f, ok := toFloat64(v)
	if !ok {
		return 0, fmt.Errorf("expected value is not a number")
	}
	return f, nil
}

func deepEqual(a, b any) bool {
	// Handle string comparison with type coercion
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	if aStr == bStr {
		return true
	}

	return reflect.DeepEqual(a, b)
}
