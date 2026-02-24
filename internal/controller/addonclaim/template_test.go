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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func TestRenderer_Render(t *testing.T) {
	r := NewRenderer()

	t.Run("basic rendering with variables", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "cilium", Namespace: "tenant-ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "cilium"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "secret"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "cilium-tpl"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"cilium","version":"1.14.5","cluster":"my-cluster"}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  repoURL: https://helm.cilium.io/
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: kube-system
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "cilium", addon.Name)
		assert.Equal(t, "1.14.5", addon.Spec.Version)
		assert.Equal(t, "my-cluster", addon.Spec.TargetCluster)
		assert.Equal(t, "https://helm.cilium.io/", addon.Spec.RepoURL)
		assert.Equal(t, "kube-system", addon.Spec.TargetNamespace)
	})

	t.Run("sprig functions", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "MyAddon", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"MyAddon","version":"1.0.0","cluster":"cluster-1"}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" | lower }}
spec:
  repoURL: https://example.com
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: {{ index .Values.spec.variables "name" | lower }}-system
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "myaddon", addon.Name)
		assert.Equal(t, "myaddon-system", addon.Spec.TargetNamespace)
	})

	t.Run("metadata access", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "from-meta", Namespace: "meta-ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"version":"2.0.0","cluster":"c"}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.metadata.name }}
spec:
  repoURL: https://example.com
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: "{{ .Values.metadata.namespace }}"
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "from-meta", addon.Name)
		assert.Equal(t, "meta-ns", addon.Spec.TargetNamespace)
	})

	t.Run("conditional with variable dependency true", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-addon", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"dep-addon","version":"1.0.0","cluster":"c","dependency":true}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  repoURL: https://example.com
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
  {{ if index .Values.spec.variables "dependency" }}releaseName: {{ index .Values.spec.variables "name" }}-dep{{ end }}`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "dep-addon-dep", addon.Spec.ReleaseName)
	})

	t.Run("conditional with variable dependency false", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "no-dep", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"no-dep","version":"1.0.0","cluster":"c","dependency":false}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  repoURL: https://example.com
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
  {{ if index .Values.spec.variables "dependency" }}releaseName: {{ index .Values.spec.variables "name" }}-dep{{ end }}`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Empty(t, addon.Spec.ReleaseName)
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.metadata.name | unknownFunc }}`

		_, err := r.Render(tmpl, claim)
		assert.Error(t, err)
	})

	t.Run("missing field reference", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.nonExistent }}`

		_, err := r.Render(tmpl, claim)
		assert.Error(t, err)
	})

	t.Run("invalid YAML output", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `{{{bad yaml output`

		_, err := r.Render(tmpl, claim)
		assert.Error(t, err)
	})

	t.Run("complex template with variables", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "apps"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "ingress-nginx"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "cred"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "ingress-tpl"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"ingress-nginx","version":"4.9.0","cluster":"prod-cluster"}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  path: "helm-chart-sources/{{ index .Values.spec.variables "name" }}"
  repoURL: https://git.example.com/charts.git
  pluginName: helm-with-values
  releaseName: {{ index .Values.spec.variables "name" }}
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: "beget-{{ index .Values.spec.variables "name" }}"
  backend:
    type: argocd
    namespace: argocd
  variables:
    cluster_name: "{{ index .Values.spec.variables "cluster" }}"`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "ingress-nginx", addon.Name)
		assert.Equal(t, "helm-chart-sources/ingress-nginx", addon.Spec.Path)
		assert.Equal(t, "helm-with-values", addon.Spec.PluginName)
		assert.Equal(t, "ingress-nginx", addon.Spec.ReleaseName)
		assert.Equal(t, "4.9.0", addon.Spec.Version)
		assert.Equal(t, "prod-cluster", addon.Spec.TargetCluster)
		assert.Equal(t, "beget-ingress-nginx", addon.Spec.TargetNamespace)
		assert.Equal(t, "prod-cluster", addon.Spec.Variables["cluster_name"])
	})

	t.Run("nil variables", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "no-vars", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.metadata.name }}
spec:
  repoURL: https://example.com
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "no-vars", addon.Name)
	})

	t.Run("vars shortcut access", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "vars-test", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"my-addon","version":"2.0.0","cluster":"prod"}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Vars.name }}
spec:
  repoURL: https://example.com
  version: "{{ .Vars.version }}"
  targetCluster: "{{ .Vars.cluster }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "my-addon", addon.Name)
		assert.Equal(t, "2.0.0", addon.Spec.Version)
		assert.Equal(t, "prod", addon.Spec.TargetCluster)
	})

	t.Run("vars shortcut with nil variables", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "no-vars-shortcut", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.metadata.name }}
spec:
  repoURL: https://example.com
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)
		assert.Equal(t, "no-vars-shortcut", addon.Name)
	})

	t.Run("vars and values coexist", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "coexist-claim", Namespace: "test-ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"coexist","version":"3.0.0","cluster":"c"}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Vars.name }}
spec:
  repoURL: https://example.com
  version: "{{ .Vars.version }}"
  targetCluster: "{{ .Vars.cluster }}"
  targetNamespace: "{{ .Values.metadata.namespace }}"
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "coexist", addon.Name)
		assert.Equal(t, "3.0.0", addon.Spec.Version)
		assert.Equal(t, "test-ns", addon.Spec.TargetNamespace)
	})

	t.Run("nested variable values", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "nested", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Variables: &apiextensionsv1.JSON{
					Raw: []byte(`{"name":"nested-addon","config":{"replicas":3}}`),
				},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  repoURL: https://example.com
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "nested-addon", addon.Name)
	})
}

func TestParseTemplate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid template",
			input:   "{{ .Values.metadata.name }}",
			wantErr: false,
		},
		{
			name:    "invalid template",
			input:   "{{ .Values.metadata.name | badFunc }}",
			wantErr: true,
		},
		{
			name:    "empty template",
			input:   "",
			wantErr: false,
		},
		{
			name:    "template with sprig",
			input:   "{{ .Values.metadata.name | upper }}",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseTemplate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}
			assert.NoError(t, err)
		})
	}
}
