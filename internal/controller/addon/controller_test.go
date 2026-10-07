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

package addon

import (
	"context"
	"testing"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/yaml"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/argocd"
	"addons-operator/internal/controller/conditions"
	"addons-operator/internal/controller/sources"
	"addons-operator/internal/controller/values"
)

// The informer cache can still serve the pre-update Application, whose comparedTo
// matches its old spec. Synced must come from the written object instead.
func TestReconcile_SyncedIgnoresStaleCachedApplication(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, addonsv1alpha1.AddToScheme(scheme))
	require.NoError(t, argocdv1alpha1.AddToScheme(scheme))

	phaseSelectors := []addonsv1alpha1.ValuesSelector{{
		Name:     "phase",
		Priority: 10,
		MatchLabels: map[string]string{
			addonLabelKey:              "demo",
			"addons.in-cloud.io/phase": "true",
		},
	}}
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Finalizers: []string{finalizerName}},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://charts.example.com",
			Version:         "1.0.0",
			TargetCluster:   "in-cluster",
			TargetNamespace: "default",
			Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
		},
		Status: addonsv1alpha1.AddonStatus{
			PhaseValuesSelector: phaseSelectors,
			ValuesHash:          "before-phase",
		},
	}
	phaseValue := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-phase", Labels: phaseSelectors[0].MatchLabels},
		Spec:       addonsv1alpha1.AddonValueSpec{Values: "phaseKey: phase-value\n"},
	}

	app, err := argocd.NewApplicationBuilder().Build(addon, "argocd", map[string]any{})
	require.NoError(t, err)
	app.Status.Sync.Status = argocdv1alpha1.SyncStatusCodeSynced
	app.Status.Sync.ComparedTo.Source = *app.Spec.Source
	app.Status.Health.Status = health.HealthStatusHealthy

	store := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(addon, phaseValue, app).
		WithStatusSubresource(&addonsv1alpha1.Addon{}).
		Build()

	cached := &argocdv1alpha1.Application{}
	require.NoError(t, store.Get(ctx, client.ObjectKeyFromObject(app), cached))

	laggingCache := interceptor.NewClient(store, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if out, ok := obj.(*argocdv1alpha1.Application); ok {
				cached.DeepCopyInto(out)

				return nil
			}

			return c.Get(ctx, key, obj, opts...)
		},
	})

	r := &AddonReconciler{
		Client:         laggingCache,
		Scheme:         scheme,
		Recorder:       record.NewFakeRecorder(16),
		templateEngine: sources.NewTemplateEngine(),
	}
	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo"}})
	require.NoError(t, err)

	written := &argocdv1alpha1.Application{}
	require.NoError(t, store.Get(ctx, client.ObjectKeyFromObject(app), written))
	var writtenValues map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(written.Spec.Source.Helm.Values), &writtenValues))
	require.Equal(t, "phase-value", writtenValues["phaseKey"], "Application spec must carry the phase values")

	got := &addonsv1alpha1.Addon{}
	require.NoError(t, store.Get(ctx, client.ObjectKeyFromObject(addon), got))
	assert.False(t, meta.IsStatusConditionTrue(got.Status.Conditions, conditions.TypeSynced),
		"Synced must not be derived from the stale cached Application")
	assert.Equal(t, values.SelectorsHash(phaseSelectors), got.Status.ObservedPhaseValuesSelectorHash)
}
