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

package argocd

import (
	"testing"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func TestApplicationBuilder_Build(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cilium",
			UID:  types.UID("test-uid-12345"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "cilium",
			RepoURL:         "https://helm.cilium.io/",
			Version:         "1.14.0",
			TargetNamespace: "kube-system",
			TargetCluster:   "in-cluster",
		},
	}

	values := map[string]any{
		"ipam": map[string]any{"mode": "kubernetes"},
		"tls":  map[string]any{"enabled": true},
	}

	app, err := builder.Build(addon, "argocd", values)
	require.NoError(t, err)

	// Verify basic metadata
	assert.Equal(t, "cilium", app.Name)
	assert.Equal(t, "argocd", app.Namespace)
	assert.Equal(t, "Application", app.Kind)
	assert.Equal(t, "argoproj.io/v1alpha1", app.APIVersion)

	// Verify labels
	assert.Equal(t, "addon-operator", app.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "cilium", app.Labels["addons.in-cloud.io/addon"])

	// Verify owner reference (uses constants, not TypeMeta)
	require.Len(t, app.OwnerReferences, 1)
	ownerRef := app.OwnerReferences[0]
	assert.Equal(t, "addons.in-cloud.io/v1alpha1", ownerRef.APIVersion)
	assert.Equal(t, "Addon", ownerRef.Kind)
	assert.Equal(t, "cilium", ownerRef.Name)
	assert.Equal(t, types.UID("test-uid-12345"), ownerRef.UID)
	assert.True(t, *ownerRef.Controller)
	assert.True(t, *ownerRef.BlockOwnerDeletion)

	// Verify source
	assert.Equal(t, "cilium", app.Spec.Source.Chart)
	assert.Equal(t, "https://helm.cilium.io/", app.Spec.Source.RepoURL)
	assert.Equal(t, "1.14.0", app.Spec.Source.TargetRevision)
	assert.NotNil(t, app.Spec.Source.Helm)
	assert.Contains(t, app.Spec.Source.Helm.Values, "ipam:")
	assert.Contains(t, app.Spec.Source.Helm.Values, "tls:")

	// Verify destination
	assert.Equal(t, "https://kubernetes.default.svc", app.Spec.Destination.Server)
	assert.Equal(t, "kube-system", app.Spec.Destination.Namespace)

	// Verify project
	assert.Equal(t, "default", app.Spec.Project)
}

func TestApplicationBuilder_Build_WithCustomProject(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				Project: "custom-project",
			},
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	assert.Equal(t, "custom-project", app.Spec.Project)
}

func TestApplicationBuilder_Build_WithExternalCluster(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "https://external-cluster.example.com:6443",
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	assert.Equal(t, "https://external-cluster.example.com:6443", app.Spec.Destination.Server)
}

func TestApplicationBuilder_Build_WithSyncPolicy(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				SyncPolicy: &addonsv1alpha1.SyncPolicy{
					Automated: &addonsv1alpha1.AutomatedSync{
						Prune:      true,
						SelfHeal:   true,
						AllowEmpty: false,
					},
					SyncOptions: []string{
						"CreateNamespace=true",
						"PrunePropagationPolicy=foreground",
					},
				},
			},
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	require.NotNil(t, app.Spec.SyncPolicy)
	require.NotNil(t, app.Spec.SyncPolicy.Automated)
	assert.True(t, app.Spec.SyncPolicy.Automated.Prune)
	assert.True(t, app.Spec.SyncPolicy.Automated.SelfHeal)
	assert.False(t, app.Spec.SyncPolicy.Automated.AllowEmpty)

	require.Len(t, app.Spec.SyncPolicy.SyncOptions, 2)
	assert.Contains(t, []string(app.Spec.SyncPolicy.SyncOptions), "CreateNamespace=true")
	assert.Contains(t, []string(app.Spec.SyncPolicy.SyncOptions), "PrunePropagationPolicy=foreground")
}

func TestApplicationBuilder_Build_WithNilSyncPolicy(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	assert.Nil(t, app.Spec.SyncPolicy)
}

func TestApplicationBuilder_Build_EmptyValues(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
		},
	}

	app, err := builder.Build(addon, "argocd", map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "{}\n", app.Spec.Source.Helm.Values)
}

func TestApplicationBuilder_UpdateSpec(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "2.0.0", // Updated version
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
		},
	}

	// Existing application with old spec
	existing := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-addon",
			Namespace:       "argocd",
			ResourceVersion: "12345", // Should be preserved
		},
		Spec: argocdv1alpha1.ApplicationSpec{
			Source: &argocdv1alpha1.ApplicationSource{
				Chart:          "test-chart",
				RepoURL:        "https://example.com/charts",
				TargetRevision: "1.0.0", // Old version
			},
		},
	}

	newValues := map[string]any{"key": "value"}
	err := builder.UpdateSpec(existing, addon, "argocd", newValues)
	require.NoError(t, err)

	// Verify spec was updated
	assert.Equal(t, "2.0.0", existing.Spec.Source.TargetRevision)
	assert.Contains(t, existing.Spec.Source.Helm.Values, "key: value")

	// Verify metadata was preserved
	assert.Equal(t, "12345", existing.ResourceVersion)
	assert.Equal(t, "test-addon", existing.Name)

	// Verify labels were added
	assert.Equal(t, "addon-operator", existing.Labels["app.kubernetes.io/managed-by"])
}

func TestApplicationBuilder_GetApplicationRef(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-addon",
		},
	}

	ref := builder.GetApplicationRef(addon, "argocd")

	assert.Equal(t, "my-addon", ref.Name)
	assert.Equal(t, "argocd", ref.Namespace)
}

func TestApplicationBuilder_GetDestinationServer(t *testing.T) {
	builder := NewApplicationBuilder()

	tests := []struct {
		name           string
		targetCluster  string
		expectedServer string
	}{
		{
			name:           "in-cluster destination",
			targetCluster:  "in-cluster",
			expectedServer: "https://kubernetes.default.svc",
		},
		{
			name:           "external cluster URL",
			targetCluster:  "https://cluster.example.com:6443",
			expectedServer: "https://cluster.example.com:6443",
		},
		{
			name:           "empty target cluster",
			targetCluster:  "",
			expectedServer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addon := &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					TargetCluster: tt.targetCluster,
				},
			}
			result := builder.getDestinationServer(addon)
			assert.Equal(t, tt.expectedServer, result)
		})
	}
}

func TestApplicationBuilder_GetProject(t *testing.T) {
	builder := NewApplicationBuilder()

	tests := []struct {
		name            string
		backendProject  string
		expectedProject string
	}{
		{
			name:            "custom project",
			backendProject:  "my-project",
			expectedProject: "my-project",
		},
		{
			name:            "empty project uses default",
			backendProject:  "",
			expectedProject: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addon := &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Backend: addonsv1alpha1.BackendSpec{
						Project: tt.backendProject,
					},
				},
			}
			result := builder.getProject(addon)
			assert.Equal(t, tt.expectedProject, result)
		})
	}
}

func TestApplicationBuilder_Build_WithManagedNamespaceMetadata(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				SyncPolicy: &addonsv1alpha1.SyncPolicy{
					Automated: &addonsv1alpha1.AutomatedSync{
						Prune:    true,
						SelfHeal: true,
					},
					SyncOptions: []string{"CreateNamespace=true"},
					ManagedNamespaceMetadata: &addonsv1alpha1.ManagedNamespaceMetadata{
						Labels: map[string]string{
							"environment": "production",
							"managed-by":  "addon-operator",
						},
						Annotations: map[string]string{
							"description": "Created by addon operator",
						},
					},
				},
			},
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	require.NotNil(t, app.Spec.SyncPolicy)
	require.NotNil(t, app.Spec.SyncPolicy.ManagedNamespaceMetadata)

	// Verify labels
	assert.Equal(t, "production", app.Spec.SyncPolicy.ManagedNamespaceMetadata.Labels["environment"])
	assert.Equal(t, "addon-operator", app.Spec.SyncPolicy.ManagedNamespaceMetadata.Labels["managed-by"])

	// Verify annotations
	assert.Equal(t, "Created by addon operator", app.Spec.SyncPolicy.ManagedNamespaceMetadata.Annotations["description"])
}

func TestApplicationBuilder_Build_WithIgnoreDifferences(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				IgnoreDifferences: []addonsv1alpha1.ResourceIgnoreDifferences{
					{
						Group:        "admissionregistration.k8s.io",
						Kind:         "ValidatingWebhookConfiguration",
						JSONPointers: []string{"/webhooks/0/failurePolicy"},
					},
					{
						Kind:                  "Deployment",
						Name:                  "my-deployment",
						Namespace:             "default",
						JQPathExpressions:     []string{".spec.replicas"},
						ManagedFieldsManagers: []string{"kubectl"},
					},
				},
			},
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	require.Len(t, app.Spec.IgnoreDifferences, 2)

	// Verify first rule
	assert.Equal(t, "admissionregistration.k8s.io", app.Spec.IgnoreDifferences[0].Group)
	assert.Equal(t, "ValidatingWebhookConfiguration", app.Spec.IgnoreDifferences[0].Kind)
	assert.Equal(t, []string{"/webhooks/0/failurePolicy"}, app.Spec.IgnoreDifferences[0].JSONPointers)

	// Verify second rule
	assert.Equal(t, "", app.Spec.IgnoreDifferences[1].Group)
	assert.Equal(t, "Deployment", app.Spec.IgnoreDifferences[1].Kind)
	assert.Equal(t, "my-deployment", app.Spec.IgnoreDifferences[1].Name)
	assert.Equal(t, "default", app.Spec.IgnoreDifferences[1].Namespace)
	assert.Equal(t, []string{".spec.replicas"}, app.Spec.IgnoreDifferences[1].JQPathExpressions)
	assert.Equal(t, []string{"kubectl"}, app.Spec.IgnoreDifferences[1].ManagedFieldsManagers)
}

func TestApplicationBuilder_GetIgnoreDifferences_EmptySlice(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				IgnoreDifferences: []addonsv1alpha1.ResourceIgnoreDifferences{},
			},
		},
	}

	app, err := builder.Build(addon, "argocd", nil)
	require.NoError(t, err)

	assert.Nil(t, app.Spec.IgnoreDifferences)
}

func TestApplicationBuilder_NeedsUpdate(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
		},
	}

	values := map[string]any{"key": "value"}

	t.Run("no update needed when spec matches", func(t *testing.T) {
		// Build the desired Application first
		desired, err := builder.Build(addon, "argocd", values)
		require.NoError(t, err)

		// Create existing that matches
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: desired.Spec,
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.False(t, needsUpdate, "should not need update when spec matches")
		assert.Empty(t, reason)
	})

	t.Run("needs update when version differs", func(t *testing.T) {
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: argocdv1alpha1.ApplicationSpec{
				Project: "default",
				Source: &argocdv1alpha1.ApplicationSource{
					Chart:          "test-chart",
					RepoURL:        "https://example.com/charts",
					TargetRevision: "0.9.0", // Different version
					Helm:           &argocdv1alpha1.ApplicationSourceHelm{Values: "key: value\n"},
				},
				Destination: argocdv1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test-ns",
				},
			},
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when version differs")
		assert.Contains(t, reason, "targetRevision differs")
	})

	t.Run("needs update when helm values differ", func(t *testing.T) {
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: argocdv1alpha1.ApplicationSpec{
				Project: "default",
				Source: &argocdv1alpha1.ApplicationSource{
					Chart:          "test-chart",
					RepoURL:        "https://example.com/charts",
					TargetRevision: "1.0.0",
					Helm:           &argocdv1alpha1.ApplicationSourceHelm{Values: "different: values\n"},
				},
				Destination: argocdv1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test-ns",
				},
			},
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when helm values differ")
		assert.Contains(t, reason, "helm values differ")
	})

	t.Run("needs update when label missing", func(t *testing.T) {
		desired, err := builder.Build(addon, "argocd", values)
		require.NoError(t, err)

		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels:    map[string]string{}, // Missing labels
			},
			Spec: desired.Spec,
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when labels missing")
		assert.Contains(t, reason, "label")
	})
}

func TestApplicationBuilder_NeedsUpdate_IgnoreDifferencesChanged(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				IgnoreDifferences: []addonsv1alpha1.ResourceIgnoreDifferences{
					{
						Group:        "admissionregistration.k8s.io",
						Kind:         "ValidatingWebhookConfiguration",
						JSONPointers: []string{"/webhooks/0/failurePolicy"},
					},
				},
			},
		},
	}

	t.Run("needs update when ignoreDifferences added", func(t *testing.T) {
		values := map[string]any{}
		// Existing application without ignoreDifferences
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: argocdv1alpha1.ApplicationSpec{
				Project: "default",
				Source: &argocdv1alpha1.ApplicationSource{
					Chart:          "test-chart",
					RepoURL:        "https://example.com/charts",
					TargetRevision: "1.0.0",
					Helm:           &argocdv1alpha1.ApplicationSourceHelm{Values: "{}\n"},
				},
				Destination: argocdv1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test-ns",
				},
				IgnoreDifferences: nil, // No ignoreDifferences
			},
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when ignoreDifferences added")
		assert.Contains(t, reason, "ignoreDifferences differs")
	})

	t.Run("needs update when ignoreDifferences changed", func(t *testing.T) {
		values := map[string]any{}
		// Existing application with different ignoreDifferences
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: argocdv1alpha1.ApplicationSpec{
				Project: "default",
				Source: &argocdv1alpha1.ApplicationSource{
					Chart:          "test-chart",
					RepoURL:        "https://example.com/charts",
					TargetRevision: "1.0.0",
					Helm:           &argocdv1alpha1.ApplicationSourceHelm{Values: "{}\n"},
				},
				Destination: argocdv1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test-ns",
				},
				IgnoreDifferences: []argocdv1alpha1.ResourceIgnoreDifferences{
					{
						Kind:         "Deployment", // Different kind
						JSONPointers: []string{"/spec/replicas"},
					},
				},
			},
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when ignoreDifferences changed")
		assert.Contains(t, reason, "ignoreDifferences differs")
	})

	t.Run("no update needed when ignoreDifferences match", func(t *testing.T) {
		values := map[string]any{}
		desired, err := builder.Build(addon, "argocd", values)
		require.NoError(t, err)

		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: desired.Spec,
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.False(t, needsUpdate, "should not need update when ignoreDifferences match")
		assert.Empty(t, reason)
	})
}

func TestApplicationBuilder_NeedsUpdate_ManagedNamespaceMetadataChanged(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://example.com/charts",
			Version:         "1.0.0",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
			Backend: addonsv1alpha1.BackendSpec{
				SyncPolicy: &addonsv1alpha1.SyncPolicy{
					ManagedNamespaceMetadata: &addonsv1alpha1.ManagedNamespaceMetadata{
						Labels: map[string]string{"env": "prod"},
					},
				},
			},
		},
	}

	t.Run("needs update when managedNamespaceMetadata added", func(t *testing.T) {
		values := map[string]any{}
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: argocdv1alpha1.ApplicationSpec{
				Project: "default",
				Source: &argocdv1alpha1.ApplicationSource{
					Chart:          "test-chart",
					RepoURL:        "https://example.com/charts",
					TargetRevision: "1.0.0",
					Helm:           &argocdv1alpha1.ApplicationSourceHelm{Values: "{}\n"},
				},
				Destination: argocdv1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test-ns",
				},
				SyncPolicy: &argocdv1alpha1.SyncPolicy{
					// No ManagedNamespaceMetadata
				},
			},
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when managedNamespaceMetadata added")
		assert.Contains(t, reason, "syncPolicy differs")
	})
}

func TestApplicationBuilder_Build_WithPath(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
			UID:  types.UID("test-uid-path"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Path:            "charts/my-app",
			RepoURL:         "https://github.com/org/repo.git",
			Version:         "main",
			TargetNamespace: "my-app",
			TargetCluster:   "in-cluster",
		},
	}

	values := map[string]any{
		"replicaCount": 3,
	}

	app, err := builder.Build(addon, "argocd", values)
	require.NoError(t, err)

	// Verify source uses path, not chart
	assert.Equal(t, "", app.Spec.Source.Chart)
	assert.Equal(t, "charts/my-app", app.Spec.Source.Path)
	assert.Equal(t, "https://github.com/org/repo.git", app.Spec.Source.RepoURL)
	assert.Equal(t, "main", app.Spec.Source.TargetRevision)
	assert.NotNil(t, app.Spec.Source.Helm)
	assert.Contains(t, app.Spec.Source.Helm.Values, "replicaCount: 3")
}

func TestApplicationBuilder_NeedsUpdate_PathChanged(t *testing.T) {
	builder := NewApplicationBuilder()

	addon := &addonsv1alpha1.Addon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "addons.in-cloud.io/v1alpha1",
			Kind:       "Addon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-addon",
			UID:  types.UID("test-uid"),
		},
		Spec: addonsv1alpha1.AddonSpec{
			Path:            "charts/v2",
			RepoURL:         "https://github.com/org/repo.git",
			Version:         "main",
			TargetNamespace: "test-ns",
			TargetCluster:   "in-cluster",
		},
	}

	values := map[string]any{}

	t.Run("needs update when path differs", func(t *testing.T) {
		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: argocdv1alpha1.ApplicationSpec{
				Project: "default",
				Source: &argocdv1alpha1.ApplicationSource{
					Path:           "charts/v1", // Different path
					RepoURL:        "https://github.com/org/repo.git",
					TargetRevision: "main",
					Helm:           &argocdv1alpha1.ApplicationSourceHelm{Values: "{}\n"},
				},
				Destination: argocdv1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test-ns",
				},
			},
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.True(t, needsUpdate, "should need update when path differs")
		assert.Contains(t, reason, "path differs")
	})

	t.Run("no update needed when path matches", func(t *testing.T) {
		desired, err := builder.Build(addon, "argocd", values)
		require.NoError(t, err)

		existing := &argocdv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-addon",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "addon-operator",
					"addons.in-cloud.io/addon":     "test-addon",
				},
			},
			Spec: desired.Spec,
		}

		needsUpdate, reason, err := builder.NeedsUpdate(existing, addon, "argocd", values)
		require.NoError(t, err)
		assert.False(t, needsUpdate, "should not need update when path matches")
		assert.Empty(t, reason)
	})
}
