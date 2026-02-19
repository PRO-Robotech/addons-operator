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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddonClaimSpec defines the desired state of an AddonClaim.
// +kubebuilder:validation:XValidation:rule="!(has(self.values) && size(self.valuesString) > 0)",message="values and valuesString are mutually exclusive"
type AddonClaimSpec struct {
	// Name is the addon name. Used as the Addon resource name in the infra cluster.
	// Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Version is the addon version.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Version string `json:"version"`

	// Cluster is the target client cluster name (used in template rendering).
	// Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="cluster is immutable"
	Cluster string `json:"cluster"`

	// CredentialRef references the Secret containing the infra cluster kubeconfig.
	// The Secret must have a "value" key with the kubeconfig data.
	// +kubebuilder:validation:Required
	CredentialRef CredentialRef `json:"credentialRef"`

	// TemplateRef references the AddonTemplate to use for rendering.
	// +kubebuilder:validation:Required
	TemplateRef TemplateRef `json:"templateRef"`

	// Values provides structured Helm values as a JSON object.
	// Mutually exclusive with ValuesString — only one may be set.
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// ValuesString provides Helm values as a YAML string.
	// May contain Go template expressions rendered with the AddonClaim context.
	// Mutually exclusive with Values — only one may be set.
	// +optional
	ValuesString string `json:"valuesString,omitempty"`

	// Dependency marks this addon as a dependency for devops tooling.
	// When true, the annotation dependency.addons.in-cloud.io/enabled is set on the Addon.
	// +optional
	Dependency bool `json:"dependency,omitempty"`
}

// CredentialRef references a Secret with cluster credentials.
type CredentialRef struct {
	// Name of the Secret containing the kubeconfig (key: "value").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// TemplateRef references an AddonTemplate.
type TemplateRef struct {
	// Name of the AddonTemplate.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// AddonClaimStatus defines the observed state of an AddonClaim.
type AddonClaimStatus struct {
	// ObservedGeneration is the last spec.generation processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Ready indicates the remote Addon is fully reconciled and healthy.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Deployed indicates the remote Addon has been deployed at least once.
	// +optional
	Deployed bool `json:"deployed,omitempty"`

	// RemoteAddonStatus mirrors the status of the Addon in the infra cluster.
	// +optional
	RemoteAddonStatus *RemoteAddonStatus `json:"remoteAddonStatus,omitempty"`

	// Conditions represent the current state of the AddonClaim.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RemoteAddonStatus contains a snapshot of the Addon status from the infra cluster.
type RemoteAddonStatus struct {
	// Deployed indicates the remote Addon has been deployed.
	// +optional
	Deployed bool `json:"deployed,omitempty"`

	// Conditions from the remote Addon.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Addon",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AddonClaim represents a request to deploy an addon in an infra cluster.
// The controller renders the referenced AddonTemplate with the claim's values
// and creates the corresponding Addon and AddonValue in the remote cluster.
type AddonClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec AddonClaimSpec `json:"spec"`

	// +optional
	Status AddonClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddonClaimList contains a list of AddonClaim resources.
type AddonClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonClaim{}, &AddonClaimList{})
}
