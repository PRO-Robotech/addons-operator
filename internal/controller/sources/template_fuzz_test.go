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
	"testing"
)

// FuzzTemplateApply tests the template engine with random inputs.
// Run with: go test -fuzz=FuzzTemplateApply -fuzztime=30s ./internal/controller/sources/
func FuzzTemplateApply(f *testing.F) {
	// Seed corpus with various template patterns
	seeds := []string{
		// Simple variable substitution
		`key: "{{ .Variables.foo }}"`,
		// Nested access
		`nested: "{{ .Values.config.name }}"`,
		// Range over variables
		`list: "{{ range $k, $v := .Variables }}{{ $k }}={{ $v }},{{ end }}"`,
		// Conditional
		`cond: "{{ if .Variables.enabled }}yes{{ else }}no{{ end }}"`,
		// Sprig functions
		`upper: "{{ .Variables.name | upper }}"`,
		`default: "{{ .Variables.missing | default \"fallback\" }}"`,
		// Malformed templates (should error, not panic)
		`broken: "{{ .Variables.foo"`,
		`unclosed: "{{ if .Variables.x }}"`,
		// Edge cases
		`empty: ""`,
		`noop: "plain text"`,
		`nested: "{{ .Variables.a }}{{ .Variables.b }}"`,
		// Deep nesting
		`deep: "{{ .Values.a.b.c.d.e }}"`,
	}

	for _, s := range seeds {
		f.Add(s)
	}

	engine := NewTemplateEngine()
	ctx := TemplateContext{
		Variables: map[string]string{
			"foo":     "bar",
			"name":    "test",
			"enabled": "true",
			"a":       "valueA",
			"b":       "valueB",
		},
		Values: map[string]any{
			"config": map[string]any{
				"name": "testconfig",
			},
			"a": map[string]any{
				"b": map[string]any{
					"c": map[string]any{
						"d": map[string]any{
							"e": "deepvalue",
						},
					},
				},
			},
		},
	}

	f.Fuzz(func(t *testing.T, tmplStr string) {
		// Create values map containing the template string
		values := map[string]any{
			"rendered": tmplStr,
		}

		// The function should not panic regardless of input
		// Errors are expected for malformed templates, that's OK
		_, _ = engine.Apply(values, ctx)
	})
}

// FuzzTemplateCache tests the template cache with random inputs.
func FuzzTemplateCache(f *testing.F) {
	seeds := []string{
		"{{ . }}",
		"{{ .Variables }}",
		"plain text",
		"",
		"{{ range . }}{{ . }}{{ end }}",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, content string) {
		cache := newTemplateCache()

		// First call
		tmpl1, err1 := cache.getOrParse(content)

		// Second call (should hit cache)
		tmpl2, err2 := cache.getOrParse(content)

		// Both should have same result
		if (err1 == nil) != (err2 == nil) {
			t.Errorf("inconsistent cache behavior: first error=%v, second error=%v", err1, err2)
		}

		// If both successful, templates should be same instance (pointer equality)
		if err1 == nil && err2 == nil && tmpl1 != tmpl2 {
			t.Error("cache returned different templates for same content")
		}
	})
}
