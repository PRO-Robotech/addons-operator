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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func TestRenderer_Render(t *testing.T) {
	r := NewRenderer()

	t.Run("basic rendering", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "cilium", Namespace: "tenant-ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Name:          "cilium",
				Version:       "1.14.5",
				Cluster:       "my-cluster",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "secret"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "cilium-tpl"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  repoURL: https://helm.cilium.io/
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
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
				Name:          "MyAddon",
				Version:       "1.0.0",
				Cluster:       "cluster-1",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name | lower }}
spec:
  repoURL: https://example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: {{ .Values.spec.name | lower }}-system
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
				Name:          "ignored",
				Version:       "2.0.0",
				Cluster:       "c",
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
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: "{{ .Values.metadata.namespace }}"
  backend:
    type: argocd
    namespace: argocd`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "from-meta", addon.Name)
		assert.Equal(t, "meta-ns", addon.Spec.TargetNamespace)
	})

	t.Run("conditional with dependency true", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-addon", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Name:          "dep-addon",
				Version:       "1.0.0",
				Cluster:       "c",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Dependency:    true,
			},
		}

		// dependency is omitempty, so use index for safe access
		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  repoURL: https://example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
  {{ if index .Values.spec "dependency" }}releaseName: {{ .Values.spec.name }}-dep{{ end }}`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Equal(t, "dep-addon-dep", addon.Spec.ReleaseName)
	})

	t.Run("conditional with dependency false", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "no-dep", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Name:          "no-dep",
				Version:       "1.0.0",
				Cluster:       "c",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
				Dependency:    false,
			},
		}

		// dependency=false is zero value, omitempty drops it from JSON;
		// index returns nil for missing keys instead of erroring
		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  repoURL: https://example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
  {{ if index .Values.spec "dependency" }}releaseName: {{ .Values.spec.name }}-dep{{ end }}`

		addon, err := r.Render(tmpl, claim)
		require.NoError(t, err)

		assert.Empty(t, addon.Spec.ReleaseName)
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Name:          "test",
				Version:       "1.0.0",
				Cluster:       "c",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "s"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "t"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name | unknownFunc }}`

		_, err := r.Render(tmpl, claim)
		assert.Error(t, err)
	})

	t.Run("missing field reference", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Name:          "test",
				Version:       "1.0.0",
				Cluster:       "c",
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
				Name:          "test",
				Version:       "1.0.0",
				Cluster:       "c",
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
				Name:          "ingress-nginx",
				Version:       "4.9.0",
				Cluster:       "prod-cluster",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "cred"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "ingress-tpl"},
			},
		}

		tmpl := `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  path: "helm-chart-sources/{{ .Values.spec.name }}"
  repoURL: https://git.example.com/charts.git
  pluginName: helm-with-values
  releaseName: {{ .Values.spec.name }}
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: "beget-{{ .Values.spec.name }}"
  backend:
    type: argocd
    namespace: argocd
  variables:
    cluster_name: "{{ .Values.spec.cluster }}"`

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
}

func TestParseTemplate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid template",
			input:   "{{ .Values.spec.name }}",
			wantErr: false,
		},
		{
			name:    "invalid template",
			input:   "{{ .Values.spec.name | badFunc }}",
			wantErr: true,
		},
		{
			name:    "empty template",
			input:   "",
			wantErr: false,
		},
		{
			name:    "template with sprig",
			input:   "{{ .Values.spec.name | upper }}",
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
