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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"text/template"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Masterminds/sprig/v3"
	"sigs.k8s.io/yaml"
)

const defaultTemplateCacheSize = 1000

// TemplateContext provides data for template rendering.
type TemplateContext struct {
	// Variables from Addon.spec.variables
	Variables map[string]string

	// Values extracted from valuesSources
	Values map[string]any
}

// TemplateEngine applies Go templates to values.
type TemplateEngine interface {
	// Apply applies templates to values using the provided context.
	Apply(values map[string]any, ctx TemplateContext) (map[string]any, error)

	// RenderString renders Go templates within a raw string.
	// If the string contains no template expressions, it is returned as-is.
	RenderString(content string, ctx TemplateContext) (string, error)
}

type templateEngine struct {
	cache *templateCache
}

// NewTemplateEngine creates a new TemplateEngine instance.
func NewTemplateEngine() TemplateEngine {
	return &templateEngine{
		cache: newTemplateCache(),
	}
}

func (e *templateEngine) Apply(values map[string]any, ctx TemplateContext) (map[string]any, error) {
	if len(values) == 0 {
		return values, nil
	}

	yamlBytes, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal values: %w", err)
	}

	yamlStr := string(yamlBytes)
	if !containsTemplate(yamlStr) {
		return values, nil
	}

	tmpl, err := e.cache.getOrParse(yamlStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("result parse error: %w", err)
	}

	return result, nil
}

func (e *templateEngine) RenderString(content string, ctx TemplateContext) (string, error) {
	if !containsTemplate(content) {
		return content, nil
	}

	tmpl, err := e.cache.getOrParse(content)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}

	return buf.String(), nil
}

func containsTemplate(s string) bool {
	return bytes.Contains([]byte(s), []byte("{{"))
}

// templateCache is an LRU cache for parsed templates.
// It limits memory usage by evicting least recently used templates when full.
type templateCache struct {
	cache *lru.Cache[string, *template.Template]
}

func newTemplateCache() *templateCache {
	// LRU cache is thread-safe, no need for separate mutex
	cache, _ := lru.New[string, *template.Template](defaultTemplateCacheSize)

	return &templateCache{
		cache: cache,
	}
}

func (c *templateCache) getOrParse(content string) (*template.Template, error) {
	hash := sha256.Sum256([]byte(content))
	key := hex.EncodeToString(hash[:])

	// Check cache first (LRU cache is thread-safe)
	if tmpl, ok := c.cache.Get(key); ok {
		return tmpl, nil
	}

	// Parse template (expensive operation)
	tmpl, err := newTemplate("values", content)
	if err != nil {
		return nil, err
	}

	// Add to cache (evicts LRU entry if at capacity)
	c.cache.Add(key, tmpl)

	return tmpl, nil
}

func newTemplate(name, content string) (*template.Template, error) {
	return template.New(name).
		Option("missingkey=error").
		Funcs(sprig.FuncMap()).
		Parse(content)
}
