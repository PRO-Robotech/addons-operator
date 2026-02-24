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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

func TestSyncExternalStatus(t *testing.T) {
	tests := []struct {
		name               string
		annotations        map[string]string
		remoteAddonStatus  *addonsv1alpha1.RemoteAddonStatus
		variables          *apiextensionsv1.JSON
		wantInitialized    *bool
		wantInitialization *addonsv1alpha1.Initialization
		wantExternalCP     *bool
		wantVersion        string
	}{
		{
			name:        "Deployed=True sets Initialized=true and Version",
			annotations: map[string]string{"external-status/type": "control-plane"},
			remoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
				Conditions: []metav1.Condition{
					{Type: "Deployed", Status: metav1.ConditionTrue},
				},
			},
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			wantInitialized:    boolPtr(true),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(true)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.28.0",
		},
		{
			name:        "Deployed=False sets Initialized=false",
			annotations: map[string]string{"external-status/type": "control-plane"},
			remoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
				Conditions: []metav1.Condition{
					{Type: "Deployed", Status: metav1.ConditionFalse},
				},
			},
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			wantInitialized:    boolPtr(false),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(false)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.28.0",
		},
		{
			name:        "no Deployed condition (only Ready) sets Initialized=false",
			annotations: map[string]string{"external-status/type": "control-plane"},
			remoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue},
				},
			},
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			wantInitialized:    boolPtr(false),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(false)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.28.0",
		},
		{
			name:               "nil RemoteAddonStatus sets Initialized=false",
			annotations:        map[string]string{"external-status/type": "control-plane"},
			remoteAddonStatus:  nil,
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			wantInitialized:    boolPtr(false),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(false)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "1.28.0",
		},
		{
			name:               "without annotation clears all CAPI fields",
			annotations:        nil,
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Conditions: []metav1.Condition{{Type: "Deployed", Status: metav1.ConditionTrue}}},
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			wantInitialized:    nil,
			wantInitialization: nil,
			wantExternalCP:     nil,
			wantVersion:        "",
		},
		{
			name:        "empty annotation value clears all CAPI fields",
			annotations: map[string]string{"external-status/type": ""},
			remoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
				Conditions: []metav1.Condition{{Type: "Deployed", Status: metav1.ConditionTrue}},
			},
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			wantInitialized:    nil,
			wantInitialization: nil,
			wantExternalCP:     nil,
			wantVersion:        "",
		},
		{
			name:        "no version variable returns empty Version",
			annotations: map[string]string{"external-status/type": "control-plane"},
			remoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
				Conditions: []metav1.Condition{{Type: "Deployed", Status: metav1.ConditionTrue}},
			},
			variables:          &apiextensionsv1.JSON{Raw: []byte(`{"cluster":"prod"}`)},
			wantInitialized:    boolPtr(true),
			wantInitialization: &addonsv1alpha1.Initialization{ControlPlaneInitialized: boolPtr(true)},
			wantExternalCP:     boolPtr(true),
			wantVersion:        "",
		},
		{
			name:               "nil variables returns empty Version",
			annotations:        map[string]string{"external-status/type": "control-plane"},
			remoteAddonStatus:  &addonsv1alpha1.RemoteAddonStatus{Conditions: []metav1.Condition{{Type: "Deployed", Status: metav1.ConditionTrue}}},
			variables:          nil,
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
					Variables:     tt.variables,
				},
				Status: addonsv1alpha1.AddonClaimStatus{
					RemoteAddonStatus: tt.remoteAddonStatus,
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
				Variables:     &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			},
			Status: addonsv1alpha1.AddonClaimStatus{
				Ready: boolPtr(true),
				RemoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
					Conditions: []metav1.Condition{
						{Type: "Deployed", Status: metav1.ConditionTrue},
					},
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
				Variables:     &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			},
			Status: addonsv1alpha1.AddonClaimStatus{
				Ready: boolPtr(false),
				RemoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
					Conditions: []metav1.Condition{
						{Type: "Deployed", Status: metav1.ConditionFalse},
					},
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
				Variables:     &apiextensionsv1.JSON{Raw: []byte(`{"version":"1.28.0"}`)},
			},
			Status: addonsv1alpha1.AddonClaimStatus{
				RemoteAddonStatus: &addonsv1alpha1.RemoteAddonStatus{
					Conditions: []metav1.Condition{
						{Type: "Deployed", Status: metav1.ConditionTrue},
					},
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
