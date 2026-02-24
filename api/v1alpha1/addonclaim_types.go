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

// AddonIdentity specifies the identity of the Addon resource
// created in the remote cluster. Fields are immutable after creation.
type AddonIdentity struct {
	// Name of the Addon resource in the remote cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// AddonClaimSpec defines the desired state of an AddonClaim.
// +kubebuilder:validation:XValidation:rule="!(has(self.values) && size(self.valuesString) > 0)",message="values and valuesString are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="self.addon.name == oldSelf.addon.name",message="spec.addon.name is immutable"
type AddonClaimSpec struct {
	// Addon specifies the identity of the remote Addon resource.
	// The name is required and immutable — it cannot be changed after creation.
	// The rendered template's metadata.name is overridden by this value.
	// +kubebuilder:validation:Required
	Addon AddonIdentity `json:"addon"`

	// TemplateRef references the AddonTemplate to use for rendering.
	// +kubebuilder:validation:Required
	TemplateRef TemplateRef `json:"templateRef"`

	// CredentialRef references the Secret containing the infra cluster kubeconfig.
	// The Secret must have a "value" key with the kubeconfig data.
	// +kubebuilder:validation:Required
	CredentialRef CredentialRef `json:"credentialRef"`

	// Variables provides arbitrary parameters for template rendering.
	// Accessible in templates as .Vars.<key> (shortcut) or .Values.spec.variables.<key>.
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	Variables *apiextensionsv1.JSON `json:"variables,omitempty"`

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

	// ValueLabels overrides the "addons.in-cloud.io/values" label on the generated AddonValue.
	// Defaults to "claim". Used to match custom valuesSelectors in the rendered Addon.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:default="claim"
	ValueLabels string `json:"valueLabels,omitempty"`
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
	Ready *bool `json:"ready,omitempty"`

	// Deployed indicates the remote Addon has been deployed at least once.
	// +optional
	Deployed bool `json:"deployed,omitempty"`

	// RemoteAddonStatus mirrors the status of the Addon in the infra cluster.
	// +optional
	RemoteAddonStatus *RemoteAddonStatus `json:"remoteAddonStatus,omitempty"`

	// Initialized indicates the control plane has been initialized (CAPI v1beta1, deprecated).
	// Reflects the Deployed condition from the remote Addon.
	// Populated only when annotation "external-status/type" is present.
	// +optional
	Initialized *bool `json:"initialized,omitempty"`

	// Initialization contains CAPI v1beta2 initialization status.
	// +optional
	Initialization *Initialization `json:"initialization,omitempty"`

	// ExternalManagedControlPlane indicates the control plane is externally managed.
	// Populated only when annotation "external-status/type" is present.
	// +optional
	ExternalManagedControlPlane *bool `json:"externalManagedControlPlane,omitempty"`

	// Version reflects the Kubernetes version from the claim's variables.
	// Populated only when annotation "external-status/type" is present.
	// +optional
	Version string `json:"version,omitempty"`

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

// Initialization contains CAPI v1beta2 initialization status.
type Initialization struct {
	// ControlPlaneInitialized indicates the first control plane instance is ready.
	// +optional
	ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Addon",type=string,JSONPath=`.spec.addon.name`
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
