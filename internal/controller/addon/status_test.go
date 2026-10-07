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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/sources"
)

// AddonPhase owns status.phaseValuesSelector and may patch it after Reconcile has
// taken its cache snapshot; the Addon status write must not revert that patch.
func TestReconcile_PreservesConcurrentPhaseValuesSelector(t *testing.T) {
	phaseOld := []addonsv1alpha1.ValuesSelector{{
		Name:        "old",
		Priority:    10,
		MatchLabels: map[string]string{"addons.in-cloud.io/phase": "old"},
	}}
	phaseNew := []addonsv1alpha1.ValuesSelector{{
		Name:        "new",
		Priority:    20,
		MatchLabels: map[string]string{"addons.in-cloud.io/phase": "new"},
	}}

	tests := []struct {
		name     string
		snapshot []addonsv1alpha1.ValuesSelector
		patched  []addonsv1alpha1.ValuesSelector
	}{
		{name: "phase activates selectors", snapshot: nil, patched: phaseNew},
		{name: "phase switches selectors", snapshot: phaseOld, patched: phaseNew},
		{name: "phase deletion clears selectors", snapshot: phaseOld, patched: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, addonsv1alpha1.AddToScheme(scheme))
			require.NoError(t, argocdv1alpha1.AddToScheme(scheme))

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
				Status: addonsv1alpha1.AddonStatus{PhaseValuesSelector: tt.snapshot},
			}

			store := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(addon).
				WithStatusSubresource(&addonsv1alpha1.Addon{}).
				Build()

			phasePatched := false
			cache := interceptor.NewClient(store, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if err := c.Get(ctx, key, obj, opts...); err != nil {
						return err
					}
					if _, ok := obj.(*addonsv1alpha1.Addon); ok && !phasePatched {
						phasePatched = true
						patchPhaseValuesSelector(t, ctx, store, key, tt.patched)
					}

					return nil
				},
			})

			r := &AddonReconciler{
				Client:         cache,
				APIReader:      store,
				Scheme:         scheme,
				Recorder:       record.NewFakeRecorder(16),
				templateEngine: sources.NewTemplateEngine(),
			}
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo"}})
			require.NoError(t, err)
			require.True(t, phasePatched, "phase patch must run between the snapshot and the status write")

			got := &addonsv1alpha1.Addon{}
			require.NoError(t, store.Get(ctx, client.ObjectKeyFromObject(addon), got))
			assert.NotEmpty(t, got.Status.Conditions, "Reconcile must have written its own status")
			assert.Equal(t, tt.patched, got.Status.PhaseValuesSelector)
		})
	}
}

func patchPhaseValuesSelector(
	t *testing.T,
	ctx context.Context,
	c client.Client,
	key client.ObjectKey,
	selectors []addonsv1alpha1.ValuesSelector,
) {
	t.Helper()

	fresh := &addonsv1alpha1.Addon{}
	require.NoError(t, c.Get(ctx, key, fresh))
	patch := client.MergeFrom(fresh.DeepCopy())
	fresh.Status.PhaseValuesSelector = selectors
	require.NoError(t, c.Status().Patch(ctx, fresh, patch))
}
