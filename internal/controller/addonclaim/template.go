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

package addonclaim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"sigs.k8s.io/yaml"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// RenderContext is passed as the data to template.Execute.
// Templates access claim fields as .Values.spec.name, .Values.metadata.namespace, etc.
// .Vars provides a shortcut to spec.variables for ergonomic access (e.g., .Vars.foo).
type RenderContext struct {
	Values map[string]any
	Vars   map[string]any
}

// Renderer renders AddonTemplate templates with AddonClaim context.
type Renderer struct{}

// NewRenderer creates a new Renderer instance.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// Render executes the given Go template string with the AddonClaim as context
// and returns the resulting Addon resource.
func (r *Renderer) Render(templateStr string, claim *addonsv1alpha1.AddonClaim) (*addonsv1alpha1.Addon, error) {
	raw, err := json.Marshal(claim)
	if err != nil {
		return nil, fmt.Errorf("marshal claim to JSON: %w", err)
	}

	var claimMap map[string]any
	if err = json.Unmarshal(raw, &claimMap); err != nil {
		return nil, fmt.Errorf("unmarshal claim to map: %w", err)
	}

	var vars map[string]any
	if claim.Spec.Variables != nil && len(claim.Spec.Variables.Raw) > 0 {
		if err = json.Unmarshal(claim.Spec.Variables.Raw, &vars); err != nil {
			return nil, fmt.Errorf("unmarshal variables to map: %w", err)
		}
	}

	ctx := RenderContext{Values: claimMap, Vars: vars}

	tmpl, err := template.New("addon-template").
		Option("missingkey=error").
		Funcs(sprig.FuncMap()).
		Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	var addon addonsv1alpha1.Addon
	if err = yaml.Unmarshal(buf.Bytes(), &addon); err != nil {
		return nil, fmt.Errorf("unmarshal rendered addon: %w", err)
	}

	return &addon, nil
}

// ParseTemplate validates that the template string parses without errors.
// Used by AddonTemplate webhook for early validation.
func ParseTemplate(templateStr string) error {
	_, err := template.New("addon-template").
		Option("missingkey=error").
		Funcs(sprig.FuncMap()).
		Parse(templateStr)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	return nil
}
