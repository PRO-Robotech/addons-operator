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
	"regexp"
	"strconv"
	"strings"
)

// PathSegment represents one segment of a JSONPath expression.
type PathSegment interface {
	// Extract extracts a value from the current level of the object.
	Extract(value any) (any, bool)
}

// FieldSegment accesses a field in an object: /status/phase
type FieldSegment struct {
	Field string
}

// Extract extracts the field value from a map.
func (s FieldSegment) Extract(value any) (any, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := m[s.Field]
	return v, ok
}

// IndexSegment accesses an element in an array: /conditions/0
type IndexSegment struct {
	Index int
}

// Extract extracts the element at the specified index from an array.
func (s IndexSegment) Extract(value any) (any, bool) {
	arr, ok := value.([]any)
	if !ok || s.Index < 0 || s.Index >= len(arr) {
		return nil, false
	}
	return arr[s.Index], true
}

// BracketSegment accesses a key with special characters: /labels["app.kubernetes.io/name"]
type BracketSegment struct {
	Key string
}

// Extract extracts the value for the specified key from a map.
func (s BracketSegment) Extract(value any) (any, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := m[s.Key]
	return v, ok
}

// FilterSegment filters an array by a field condition: /selectors[?(@.name=='custom')]
type FilterSegment struct {
	Field    string
	Operator string // Only '==' supported in v1
	Value    string
}

// Extract finds the first matching element in an array.
func (s FilterSegment) Extract(value any) (any, bool) {
	arr, ok := value.([]any)
	if !ok {
		return nil, false
	}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := m[s.Field]; ok {
			if fmt.Sprintf("%v", v) == s.Value {
				return item, true // Return first match
			}
		}
	}
	return nil, false
}

// Parse parses a JSONPath string into a list of segments.
//
// Supported syntax:
//
//	/a/b/c                    → [Field(a), Field(b), Field(c)]
//	/a/0/b                    → [Field(a), Index(0), Field(b)]
//	/a["b.c"]                 → [Field(a), Bracket("b.c")]
//	/a[?(@.x=='y')]           → [Field(a), Filter(x, ==, y)]
//	/a[?(@.x=='y')]/b         → [Field(a), Filter(x, ==, y), Field(b)]
func Parse(path string) ([]PathSegment, error) {
	// Handle empty path or root
	if path == "" || path == "/" {
		return nil, nil
	}

	// Path must start with /
	if path[0] != '/' {
		return nil, fmt.Errorf("path must start with /")
	}

	var segments []PathSegment
	path = path[1:] // skip leading /

	for len(path) > 0 {
		// Determine the type of the next segment
		switch path[0] {
		case '[':
			seg, rest, err := parseBracket(path)
			if err != nil {
				return nil, err
			}
			segments = append(segments, seg)
			path = rest
		default:
			seg, rest := parseField(path)
			segments = append(segments, seg)
			path = rest
		}
	}

	return segments, nil
}

// parseField parses a simple field until / or [
func parseField(path string) (PathSegment, string) {
	end := strings.IndexAny(path, "/[")
	if end == -1 {
		end = len(path)
	}

	field := path[:end]
	rest := path[end:]

	// Skip leading / if present
	if len(rest) > 0 && rest[0] == '/' {
		rest = rest[1:]
	}

	// Check if it's an index (numeric)
	if idx, err := strconv.Atoi(field); err == nil {
		return IndexSegment{Index: idx}, rest
	}

	return FieldSegment{Field: field}, rest
}

// parseBracket parses a [...] segment
func parseBracket(path string) (PathSegment, string, error) {
	// path starts with [
	// Find matching ]
	end := findMatchingBracket(path)
	if end == -1 {
		return nil, "", fmt.Errorf("unclosed bracket")
	}

	content := path[1:end] // content inside brackets
	rest := path[end+1:]

	// Skip leading / if present
	if len(rest) > 0 && rest[0] == '/' {
		rest = rest[1:]
	}

	// Determine bracket type
	switch {
	case strings.HasPrefix(content, "?(@."):
		// Filter: [?(@.field=='value')]
		seg, err := parseFilter(content)
		if err != nil {
			return nil, "", err
		}
		return seg, rest, nil
	case strings.HasPrefix(content, "\"") || strings.HasPrefix(content, "'"):
		// Quoted key: ["key"] or ['key']
		key := strings.Trim(content, "\"'")
		return BracketSegment{Key: key}, rest, nil
	default:
		// Could be a numeric index
		if idx, err := strconv.Atoi(content); err == nil {
			return IndexSegment{Index: idx}, rest, nil
		}
		return nil, "", fmt.Errorf("invalid bracket expression: %s", content)
	}
}

// findMatchingBracket finds the index of the closing bracket.
func findMatchingBracket(path string) int {
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

// filterRegex matches filter expressions like ?(@.field=='value')
var filterRegex = regexp.MustCompile(`^\?\(@\.(\w+)(==|!=)'([^']*)'\)$`)

// parseFilter parses a filter expression.
func parseFilter(content string) (PathSegment, error) {
	matches := filterRegex.FindStringSubmatch(content)
	if matches == nil {
		return nil, fmt.Errorf("invalid filter expression: %s", content)
	}

	return FilterSegment{
		Field:    matches[1],
		Operator: matches[2],
		Value:    matches[3],
	}, nil
}
