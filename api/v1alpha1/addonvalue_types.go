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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddonValueSpec defines the desired state of AddonValue.
// It contains a fragment of Helm values that can be selected by Addon
// through label matching.
type AddonValueSpec struct {
	// Values contains the Helm values fragment as arbitrary YAML/JSON.
	// Values can include Go template expressions like {{ .Variables.key }}
	// which are rendered during value aggregation.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Required
	Values runtime.RawExtension `json:"values"`
}

// AddonValueStatus defines the observed state of AddonValue.
type AddonValueStatus struct {
	// ObservedGeneration is the last spec.generation processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AddonValue stores a fragment of Helm values that can be selected by Addon.
// Values are selected through label matching (ValuesSelector.matchLabels).
// Multiple AddonValues can be merged with priority-based override semantics.
//
// Label conventions:
//   - addons.in-cloud.io/addon: <addon-name> - associates with a specific Addon
//   - addons.in-cloud.io/values: <type> - categorizes values (default, immutable, custom)
//   - addons.in-cloud.io/feature.<name>: "true" - enables feature-specific values
type AddonValue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the values fragment.
	// +kubebuilder:validation:Required
	Spec AddonValueSpec `json:"spec"`

	// Status defines the observed state.
	// +optional
	Status AddonValueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddonValueList contains a list of AddonValue resources.
type AddonValueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonValue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonValue{}, &AddonValueList{})
}
