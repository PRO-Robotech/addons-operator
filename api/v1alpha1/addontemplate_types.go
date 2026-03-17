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
)

// AddonTemplateSpec defines the desired state of an AddonTemplate.
type AddonTemplateSpec struct {
	// Template is a Go template string that renders into an Addon YAML manifest.
	// The template receives the AddonClaim as .Values (Helm-like context):
	//   .Values.spec.name, .Values.spec.version, .Values.spec.cluster, etc.
	//   .Values.metadata.name, .Values.metadata.namespace
	// Sprig template functions are available.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Template string `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AddonTemplate defines a reusable template for generating Addon resources.
// Templates are cluster-scoped and referenced by AddonClaim via templateRef.
type AddonTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec AddonTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// AddonTemplateList contains a list of AddonTemplate resources.
type AddonTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonTemplate{}, &AddonTemplateList{})
}
