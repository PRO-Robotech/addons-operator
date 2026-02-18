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

package sources

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// Extractor extracts values from external Kubernetes resources.
type Extractor interface {
	// Extract extracts data from all valuesSources.
	// Returns a map for use in templates as .Values.
	Extract(ctx context.Context, sources []addonsv1alpha1.ValueSource) (map[string]any, error)
}

type extractor struct {
	client client.Client
}

// NewExtractor creates a new Extractor instance.
func NewExtractor(c client.Client) Extractor {
	return &extractor{client: c}
}

// Extract extracts data from all valuesSources.
func (e *extractor) Extract(ctx context.Context, sources []addonsv1alpha1.ValueSource) (map[string]any, error) {
	if len(sources) == 0 {
		return make(map[string]any), nil
	}

	result := make(map[string]any)

	for _, source := range sources {
		obj, err := e.getResource(ctx, source.SourceRef)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", source.Name, err)
		}

		for _, rule := range source.Extract {
			value, err := e.extractField(obj, rule)
			if err != nil {
				return nil, fmt.Errorf("source %s, path %s: %w", source.Name, rule.JSONPath, err)
			}
			if err := setNestedField(result, rule.As, value); err != nil {
				return nil, fmt.Errorf("source %s, set %s: %w", source.Name, rule.As, err)
			}
		}
	}

	return result, nil
}

// getResource fetches a Kubernetes resource by SourceRef.
func (e *extractor) getResource(ctx context.Context, ref addonsv1alpha1.SourceRef) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(ref.APIVersion)
	u.SetKind(ref.Kind)

	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}

	if err := e.client.Get(ctx, key, u); err != nil {
		return nil, err
	}

	return u, nil
}

func (e *extractor) extractField(obj *unstructured.Unstructured, rule addonsv1alpha1.ExtractRule) (any, error) {
	path := rule.JSONPath
	if !strings.HasPrefix(path, ".") {
		return nil, fmt.Errorf("jsonPath must start with '.': %s", path)
	}

	value, found, err := extractByPath(obj.Object, path)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", path, err)
	}
	if !found {
		return nil, fmt.Errorf("path %s not found", path)
	}

	if rule.Decode != "" {
		return decodeValue(value, rule.Decode)
	}

	return value, nil
}

func extractByPath(obj any, path string) (any, bool, error) {
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return obj, true, nil
	}

	var segment string
	var rest string

	if strings.HasPrefix(path, "[") {
		end := findClosingBracket(path)
		if end == -1 {
			return nil, false, fmt.Errorf("unclosed bracket in path")
		}
		segment = path[1:end]
		rest = path[end+1:]
		segment = strings.Trim(segment, "\"'")
	} else {
		dotIdx := strings.Index(path, ".")
		bracketIdx := strings.Index(path, "[")

		switch {
		case dotIdx == -1 && bracketIdx == -1:
			segment = path
			rest = ""
		case dotIdx == -1:
			segment = path[:bracketIdx]
			rest = path[bracketIdx:]
		case bracketIdx == -1:
			segment = path[:dotIdx]
			rest = path[dotIdx:]
		case dotIdx < bracketIdx:
			segment = path[:dotIdx]
			rest = path[dotIdx:]
		default:
			segment = path[:bracketIdx]
			rest = path[bracketIdx:]
		}
	}

	switch v := obj.(type) {
	case map[string]any:
		val, ok := v[segment]
		if !ok {
			return nil, false, nil
		}
		if rest == "" || rest == "." {
			return val, true, nil
		}
		return extractByPath(val, rest)

	case []any:
		idx, err := strconv.Atoi(segment)
		if err != nil {
			return nil, false, fmt.Errorf("expected array index, got: %s", segment)
		}
		if idx < 0 || idx >= len(v) {
			return nil, false, nil
		}
		if rest == "" || rest == "." {
			return v[idx], true, nil
		}
		return extractByPath(v[idx], rest)

	default:
		return nil, false, nil
	}
}

func findClosingBracket(path string) int {
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(path); i++ {
		c := path[i]
		if inString {
			if c == stringChar {
				inString = false
			}
			continue
		}

		switch c {
		case '"', '\'':
			inString = true
			stringChar = c
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func decodeValue(value any, decode string) (any, error) {
	switch decode {
	case "":
		return value, nil
	case "base64":
		return decodeBase64(value)
	default:
		return nil, fmt.Errorf("unknown decode method: %s", decode)
	}
}

func decodeBase64(value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("base64 decode requires string, got %T", value)
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	return string(decoded), nil
}

func setNestedField(obj map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	current := obj

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}

		if _, ok := current[part]; !ok {
			current[part] = make(map[string]any)
		}
		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			return fmt.Errorf("path %q: key %q is %T, not a map", path, part, current[part])
		}
	}
	return nil
}

// SourceRefKey creates an index key for a SourceRef.
func SourceRefKey(ref addonsv1alpha1.SourceRef) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		ref.APIVersion,
		ref.Kind,
		ref.Namespace,
		ref.Name,
	)
}
