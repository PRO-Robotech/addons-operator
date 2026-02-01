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

package jsonpath

import (
	"fmt"
	"strings"

	rfc9535 "github.com/theory/jsonpath"
)

// ExtractString extracts a value from obj using RFC 9535 JSONPath syntax.
// Path must start with "$" (e.g., "$.status.phase").
// obj must be a JSON-compatible type (map[string]any, []any, string, float64, bool, nil).
// Returns (value, found, error). Missing path returns (nil, false, nil).
func ExtractString(obj any, path string) (any, bool, error) {
	if path == "" || path == "$" {
		return obj, true, nil
	}
	if !strings.HasPrefix(path, "$") {
		return nil, false, fmt.Errorf("path must start with '$': %q", path)
	}

	p, err := rfc9535.Parse(path)
	if err != nil {
		return nil, false, fmt.Errorf("invalid jsonPath %q: %w", path, err)
	}

	nodes := p.Select(obj)
	if len(nodes) == 0 {
		return nil, false, nil
	}

	return nodes[0], true, nil
}
