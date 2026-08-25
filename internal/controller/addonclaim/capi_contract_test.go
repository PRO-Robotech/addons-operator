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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	pkgconditions "addons-operator/pkg/conditions"
)

func TestSyncExternalStatus(t *testing.T) {
	const cpAnnotation = "external-status/type"

	tests := []struct {
		name               string
		annotations        map[string]string
		remoteAddonStatus  *addonsv1alpha1.RemoteAddonStatus
		ready              *bool
		priorVersion       string
		specVersion        string
		wantInitialized    *bool
		wantInitialization *addonsv1alpha1.Initialization
		wantExternalCP     *bool
		wantVersion        string
	}{
		{
			name:               "Ready=true publishes spec.version",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: true},
			ready:              boolPtr(true),
			specVersion:        "1.28.0",
			wantInitialized:    boolPtr(true),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(true)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.28.0",
		},
		{
			name:               "Ready=false keeps the previously published version",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: true},
			ready:              boolPtr(false),
			priorVersion:       "1.27.0",
			specVersion:        "1.28.0",
			wantInitialized:    boolPtr(true),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(true)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.27.0",
		},
		{
			name:               "Ready=false during first provisioning publishes nothing",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: false},
			ready:              boolPtr(false),
			specVersion:        "1.28.0",
			wantInitialized:    boolPtr(false),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(false)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "",
		},
		{
			name:               "Ready=nil publishes nothing",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: true},
			ready:              nil,
			specVersion:        "1.28.0",
			wantInitialized:    boolPtr(true),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(true)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "",
		},
		{
			name:               "Deployed drives Initialized independently of Ready",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: false},
			ready:              boolPtr(true),
			specVersion:        "1.28.0",
			wantInitialized:    boolPtr(false),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(false)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.28.0",
		},
		{
			name:               "nil RemoteAddonStatus sets Initialized=false",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  nil,
			ready:              nil,
			specVersion:        "1.28.0",
			wantInitialized:    boolPtr(false),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(false)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "",
		},
		{
			name:               "without annotation clears all CAPI fields",
			annotations:        nil,
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: true},
			ready:              boolPtr(true),
			priorVersion:       "1.27.0",
			specVersion:        "1.28.0",
			wantInitialized:    nil,
			wantInitialization: nil,
			wantExternalCP:     nil,
			wantVersion:        "",
		},
		{
			name:               "empty annotation value clears all CAPI fields",
			annotations:        map[string]string{cpAnnotation: ""},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: true},
			ready:              boolPtr(true),
			priorVersion:       "1.27.0",
			specVersion:        "1.28.0",
			wantInitialized:    nil,
			wantInitialization: nil,
			wantExternalCP:     nil,
			wantVersion:        "",
		},
		{
			name:               "empty spec.version publishes nothing",
			annotations:        map[string]string{cpAnnotation: "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Deployed: true},
			ready:              boolPtr(true),
			specVersion:        "",
			wantInitialized:    boolPtr(true),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(true)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "",
		},
	}

	r := &Reconciler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claim := &addonsv1alpha1.AddonClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-claim",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
				Spec: addonsv1alpha1.AddonClaimSpec{
					Addon:         addonsv1alpha1.AddonIdentity{Name: "test-addon"},
					TemplateRef:   addonsv1alpha1.TemplateRef{Name: "tpl"},
					CredentialRef: addonsv1alpha1.CredentialRef{Name: "cred"},
					Version:       tt.specVersion,
				},
				Status: addonsv1alpha1.AddonClaimStatus{
					RemoteAddonStatus: tt.remoteAddonStatus,
					Ready:             tt.ready,
					Version:           tt.priorVersion,
				},
			}

			r.syncExternalStatus(claim)

			assert.Equal(t, tt.wantInitialized, claim.Status.Initialized, "Initialized")
			assert.Equal(t, tt.wantInitialization, claim.Status.Initialization, "Initialization")
			assert.Equal(t, tt.wantExternalCP, claim.Status.ExternalManagedControlPlane, "ExternalManagedControlPlane")
			assert.Equal(t, tt.wantVersion, claim.Status.Version, "Version")
		})
	}
}

func TestIsAddonReadyAtCurrentGeneration(t *testing.T) {
	readyCondition := []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}

	tests := []struct {
		name               string
		generation         int64
		observedGeneration int64
		conditions         []metav1.Condition
		want               bool
	}{
		{
			name:               "Ready at the observed generation",
			generation:         3,
			observedGeneration: 3,
			conditions:         readyCondition,
			want:               true,
		},
		{
			name:               "Ready latched from the previous revision is not trusted",
			generation:         4,
			observedGeneration: 3,
			conditions:         readyCondition,
			want:               false,
		},
		{
			name:               "not Ready at the observed generation",
			generation:         3,
			observedGeneration: 3,
			conditions:         []metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse}},
			want:               false,
		},
		{
			name:               "no conditions yet",
			generation:         1,
			observedGeneration: 1,
			want:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "test-addon", Generation: tt.generation},
				Status: addonsv1alpha1.AddonStatus{
					ObservedGeneration: tt.observedGeneration,
					Conditions:         tt.conditions,
				},
			}

			assert.Equal(t, tt.want, isAddonReadyAtCurrentGeneration(addon))
		})
	}
}

func TestCAPIContractJSONPaths(t *testing.T) {
	t.Run("initialized claim exposes correct JSON paths for CAPI", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "capi-claim",
				Namespace:   "default",
				Annotations: map[string]string{"external-status/type": "control-plane"},
			},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "capi-addon"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "tpl"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "cred"},
				Version:       "1.28.0",
			},
			Status: addonsv1alpha1.AddonClaimStatus{
				Ready: boolPtr(true),
				RemoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
					Deployed: true,
				},
			},
		}

		r := &Reconciler{}
		r.syncExternalStatus(claim)

		data := claimToUnstructured(t, claim)
		status := nestedMap(t, data, "status")

		// status.initialized — CAPI v1beta1
		initialized, found, err := unstructured.NestedBool(status, "initialized")
		require.NoError(t, err)
		assert.True(t, found, "status.initialized must be present")
		assert.True(t, initialized)

		// status.ready — *bool, false must be present (not absent)
		ready, found, err := unstructured.NestedBool(status, "ready")
		require.NoError(t, err)
		assert.True(t, found, "status.ready must be present")
		assert.True(t, ready)

		// status.externalManagedControlPlane
		extCP, found, err := unstructured.NestedBool(status, "externalManagedControlPlane")
		require.NoError(t, err)
		assert.True(t, found, "status.externalManagedControlPlane must be present")
		assert.True(t, extCP)

		// status.version
		version, found, err := unstructured.NestedString(status, "version")
		require.NoError(t, err)
		assert.True(t, found, "status.version must be present")
		assert.Equal(t, "1.28.0", version)

		// status.initialization.controlPlaneInitialized — CAPI v1beta2
		cpInit, found, err := unstructured.NestedBool(status, "initialization", "controlPlaneInitialized")
		require.NoError(t, err)
		assert.True(t, found, "status.initialization.controlPlaneInitialized must be present")
		assert.True(t, cpInit)
	})

	t.Run("uninitialized claim exposes false values at correct paths", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "capi-claim-uninit",
				Namespace:   "default",
				Annotations: map[string]string{"external-status/type": "control-plane"},
			},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "capi-addon-uninit"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "tpl"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "cred"},
				Version:       "1.28.0",
			},
			Status: addonsv1alpha1.AddonClaimStatus{
				Ready: boolPtr(false),
				RemoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
					Deployed: false,
				},
			},
		}

		r := &Reconciler{}
		r.syncExternalStatus(claim)

		data := claimToUnstructured(t, claim)
		status := nestedMap(t, data, "status")

		// status.initialized = false
		initialized, found, err := unstructured.NestedBool(status, "initialized")
		require.NoError(t, err)
		assert.True(t, found, "status.initialized must be present even when false")
		assert.False(t, initialized)

		// status.ready = false — *bool ensures false is present in JSON (not omitted)
		ready, found, err := unstructured.NestedBool(status, "ready")
		require.NoError(t, err)
		assert.True(t, found, "status.ready must be present even when false")
		assert.False(t, ready)

		// status.initialization.controlPlaneInitialized = false
		cpInit, found, err := unstructured.NestedBool(status, "initialization", "controlPlaneInitialized")
		require.NoError(t, err)
		assert.True(t, found, "status.initialization.controlPlaneInitialized must be present even when false")
		assert.False(t, cpInit)

		// status.externalManagedControlPlane is still true
		extCP, found, err := unstructured.NestedBool(status, "externalManagedControlPlane")
		require.NoError(t, err)
		assert.True(t, found)
		assert.True(t, extCP)
	})

	t.Run("without annotation CAPI fields are absent from JSON", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "capi-claim-no-ann",
				Namespace: "default",
			},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: "capi-addon-no-ann"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "tpl"},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "cred"},
				Version:       "1.28.0",
			},
			Status: addonsv1alpha1.AddonClaimStatus{
				RemoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
					Deployed: true,
				},
			},
		}

		r := &Reconciler{}
		r.syncExternalStatus(claim)

		data := claimToUnstructured(t, claim)
		status := nestedMap(t, data, "status")

		_, found, err := unstructured.NestedBool(status, "initialized")
		require.NoError(t, err)
		assert.False(t, found, "status.initialized must be absent without annotation")

		// status.ready is nil (*bool) — absent from JSON
		_, found, err = unstructured.NestedBool(status, "ready")
		require.NoError(t, err)
		assert.False(t, found, "status.ready must be absent when nil")

		_, found, err = unstructured.NestedBool(status, "externalManagedControlPlane")
		require.NoError(t, err)
		assert.False(t, found, "status.externalManagedControlPlane must be absent without annotation")

		_, found, err = unstructured.NestedString(status, "version")
		require.NoError(t, err)
		assert.False(t, found, "status.version must be absent without annotation")

		_, found, err = unstructured.NestedBool(status, "initialization", "controlPlaneInitialized")
		require.NoError(t, err)
		assert.False(t, found, "status.initialization must be absent without annotation")
	})
}

func TestMirrorDeployedCondition(t *testing.T) {
	t.Run("sets Deployed condition when deployed is true", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			Status: addonsv1alpha1.AddonClaimStatus{
				Deployed: true,
			},
		}
		cm := pkgconditions.New(&claim.Status.Conditions, 1)

		r := &Reconciler{}
		r.mirrorDeployedCondition(claim, cm)

		cond := meta.FindStatusCondition(claim.Status.Conditions, "Deployed")
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, "Deployed", cond.Reason)
	})

	t.Run("does not set Deployed condition when deployed is false", func(t *testing.T) {
		claim := &addonsv1alpha1.AddonClaim{
			Status: addonsv1alpha1.AddonClaimStatus{
				Deployed: false,
			},
		}
		cm := pkgconditions.New(&claim.Status.Conditions, 1)

		r := &Reconciler{}
		r.mirrorDeployedCondition(claim, cm)

		cond := meta.FindStatusCondition(claim.Status.Conditions, "Deployed")
		assert.Nil(t, cond)
	})
}

// claimToUnstructured serializes an AddonClaim to map[string]any via JSON,
// exactly as the Kubernetes API server would expose it to CAPI.
func claimToUnstructured(t *testing.T, claim *addonsv1alpha1.AddonClaim) map[string]any {
	t.Helper()

	raw, err := json.Marshal(claim)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(raw, &data))

	return data
}

// nestedMap extracts a nested map from an unstructured object.
func nestedMap(t *testing.T, data map[string]any, fields ...string) map[string]any {
	t.Helper()

	result, found, err := unstructured.NestedMap(data, fields...)
	require.NoError(t, err)
	require.True(t, found, "expected nested map at path %v", fields)

	return result
}

func boolPtr(v bool) *bool {
	return &v
}
