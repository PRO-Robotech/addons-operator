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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateEngine_Apply(t *testing.T) {
	engine := NewTemplateEngine()

	t.Run("no templates", func(t *testing.T) {
		values := map[string]any{
			"key":   "value",
			"count": 42,
		}
		ctx := TemplateContext{}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		assert.Equal(t, values, result)
	})

	t.Run("empty values", func(t *testing.T) {
		result, err := engine.Apply(nil, TemplateContext{})
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("variables substitution", func(t *testing.T) {
		values := map[string]any{
			"cluster": map[string]any{
				"name": "{{ .Variables.cluster_name }}",
			},
			"environment": "{{ .Variables.env }}",
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"cluster_name": "production",
				"env":          "prod",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)

		cluster, ok := result["cluster"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "production", cluster["name"])
		assert.Equal(t, "prod", result["environment"])
	})

	t.Run("values from sources", func(t *testing.T) {
		values := map[string]any{
			"tls": map[string]any{
				"ca": "{{ .Values.certs.caBundle }}",
			},
			"network": map[string]any{
				"podCIDR": "{{ .Values.cluster.podCIDR }}",
			},
		}
		ctx := TemplateContext{
			Values: map[string]any{
				"certs": map[string]any{
					"caBundle": "-----BEGIN CERT-----",
				},
				"cluster": map[string]any{
					"podCIDR": "10.244.0.0/16",
				},
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)

		tls, ok := result["tls"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "-----BEGIN CERT-----", tls["ca"])

		network, ok := result["network"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "10.244.0.0/16", network["podCIDR"])
	})

	t.Run("sprig functions - upper", func(t *testing.T) {
		values := map[string]any{
			"name": "{{ .Variables.name | upper }}",
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"name": "production",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		assert.Equal(t, "PRODUCTION", result["name"])
	})

	t.Run("sprig functions - default with empty value", func(t *testing.T) {
		// Note: with missingkey=error, we can't use default for missing keys.
		// But we can use it for empty string values.
		values := map[string]any{
			"name": `{{ .Variables.name | default "unknown" }}`,
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"name": "", // empty, should use default
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		assert.Equal(t, "unknown", result["name"])
	})

	t.Run("sprig functions - quote", func(t *testing.T) {
		values := map[string]any{
			"name": "{{ .Variables.name | quote }}",
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"name": "test",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		assert.Equal(t, `"test"`, result["name"])
	})

	t.Run("sprig functions - b64enc", func(t *testing.T) {
		values := map[string]any{
			"encoded": "{{ .Variables.data | b64enc }}",
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"data": "secret",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		assert.Equal(t, "c2VjcmV0", result["encoded"]) // base64("secret")
	})

	t.Run("conditional in template - true case", func(t *testing.T) {
		values := map[string]any{
			"enabled": `{{ if eq .Variables.env "prod" }}true{{ else }}false{{ end }}`,
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"env": "prod",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		// Template output is string, YAML may or may not convert depending on quoting
		// Accept both bool true and string "true"
		switch v := result["enabled"].(type) {
		case bool:
			assert.True(t, v)
		case string:
			assert.Equal(t, "true", v)
		default:
			t.Errorf("unexpected type %T", v)
		}
	})

	t.Run("conditional in template - false case", func(t *testing.T) {
		values := map[string]any{
			"enabled": `{{ if eq .Variables.env "prod" }}true{{ else }}false{{ end }}`,
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"env": "dev",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)
		// Accept both bool false and string "false"
		switch v := result["enabled"].(type) {
		case bool:
			assert.False(t, v)
		case string:
			assert.Equal(t, "false", v)
		default:
			t.Errorf("unexpected type %T", v)
		}
	})

	t.Run("missing variable error", func(t *testing.T) {
		values := map[string]any{
			"name": "{{ .Variables.missing_var }}",
		}
		ctx := TemplateContext{
			Variables: map[string]string{},
		}

		_, err := engine.Apply(values, ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template execution error")
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		values := map[string]any{
			"bad": "{{ .Variables.name | unknownFunc }}",
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"name": "test",
			},
		}

		_, err := engine.Apply(values, ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template parse error")
	})

	t.Run("mixed templates and static", func(t *testing.T) {
		values := map[string]any{
			"static": "no template here",
			"dynamic": map[string]any{
				"name":    "{{ .Variables.name }}",
				"version": "1.0.0",
			},
			"count": 42,
		}
		ctx := TemplateContext{
			Variables: map[string]string{
				"name": "myapp",
			},
		}

		result, err := engine.Apply(values, ctx)
		require.NoError(t, err)

		assert.Equal(t, "no template here", result["static"])
		// Note: JSON/YAML unmarshal converts numbers to float64
		assert.Equal(t, float64(42), result["count"])

		dynamic, ok := result["dynamic"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "myapp", dynamic["name"])
		assert.Equal(t, "1.0.0", dynamic["version"])
	})
}

func TestTemplateEngine_RenderString(t *testing.T) {
	engine := NewTemplateEngine()

	t.Run("no templates passthrough", func(t *testing.T) {
		result, err := engine.RenderString("key: value\ncount: 42", TemplateContext{})
		require.NoError(t, err)
		assert.Equal(t, "key: value\ncount: 42", result)
	})

	t.Run("empty string", func(t *testing.T) {
		result, err := engine.RenderString("", TemplateContext{})
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("variables substitution", func(t *testing.T) {
		ctx := TemplateContext{
			Variables: map[string]string{
				"cluster_name": "production",
				"count":        "3",
			},
		}

		result, err := engine.RenderString("cluster:\n  name: {{ .Variables.cluster_name }}\nreplicas: {{ .Variables.count }}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "cluster:\n  name: production\nreplicas: 3", result)
	})

	t.Run("values from sources", func(t *testing.T) {
		ctx := TemplateContext{
			Values: map[string]any{
				"network": map[string]any{"podCIDR": "10.244.0.0/16"},
			},
		}

		result, err := engine.RenderString("podCIDR: {{ .Values.network.podCIDR }}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "podCIDR: 10.244.0.0/16", result)
	})

	t.Run("missing variable error", func(t *testing.T) {
		_, err := engine.RenderString("name: {{ .Variables.missing }}", TemplateContext{
			Variables: map[string]string{},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template execution error")
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		_, err := engine.RenderString("{{ .Variables.name | unknownFunc }}", TemplateContext{
			Variables: map[string]string{"name": "test"},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template parse error")
	})

	t.Run("sprig functions", func(t *testing.T) {
		ctx := TemplateContext{
			Variables: map[string]string{"name": "production"},
		}

		result, err := engine.RenderString("name: {{ .Variables.name | upper }}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "name: PRODUCTION", result)
	})
}

func TestTemplateCache(t *testing.T) {
	engine := NewTemplateEngine().(*templateEngine)

	values := map[string]any{
		"name": "{{ .Variables.name }}",
	}
	ctx := TemplateContext{
		Variables: map[string]string{"name": "test"},
	}

	// First call should cache
	_, err := engine.Apply(values, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, engine.cache.cache.Len())

	// Second call with same content should use cache
	_, err = engine.Apply(values, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, engine.cache.cache.Len())

	// Different content should add to cache
	values2 := map[string]any{
		"different": "{{ .Variables.name }}",
	}
	_, err = engine.Apply(values2, ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, engine.cache.cache.Len())
}

func TestContainsTemplate(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"no template", false},
		{"{{ .Variables.name }}", true},
		{"prefix {{ template }} suffix", true},
		{"{not a template}", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, containsTemplate(tt.input))
		})
	}
}

func TestTemplateCacheConcurrent(t *testing.T) {
	cache := newTemplateCache()
	content := "{{ .Variables.name }}"

	const goroutines = 100
	done := make(chan struct{})

	// Start multiple goroutines that all try to parse the same template
	for i := 0; i < goroutines; i++ {
		go func() {
			tmpl, err := cache.getOrParse(content)
			require.NoError(t, err)
			require.NotNil(t, tmpl)
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify only one template was cached
	assert.Equal(t, 1, cache.cache.Len(), "expected only one template in cache despite concurrent access")
}

func TestTemplateCacheConcurrentDifferentTemplates(t *testing.T) {
	cache := newTemplateCache()

	const goroutines = 50
	done := make(chan struct{})

	// Start multiple goroutines with different templates
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			// Use two different templates alternately
			var content string
			if idx%2 == 0 {
				content = "{{ .Variables.name }}"
			} else {
				content = "{{ .Variables.value }}"
			}
			tmpl, err := cache.getOrParse(content)
			require.NoError(t, err)
			require.NotNil(t, tmpl)
			done <- struct{}{}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify exactly two templates were cached
	assert.Equal(t, 2, cache.cache.Len(), "expected exactly two templates in cache")
}
