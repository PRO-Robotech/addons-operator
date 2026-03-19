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

// AddonSpec defines the desired state of an Addon.
// It specifies which Helm chart to deploy, where to deploy it,
// and how to aggregate values from AddonValue resources.
type AddonSpec struct {
	// Chart specifies the Helm chart name for Helm repository sources.
	// Either Chart or Path must be specified, but not both.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Chart string `json:"chart,omitempty"`

	// Path specifies the directory path within a Git repository containing the Helm chart.
	// Used when deploying from Git instead of a Helm repository.
	// Either Chart or Path must be specified, but not both.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Path string `json:"path,omitempty"`

	// RepoURL specifies the Helm repository URL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(https?|oci)://.*`
	// +kubebuilder:validation:MaxLength=2048
	RepoURL string `json:"repoURL"`

	// Version specifies the chart version to deploy.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Version string `json:"version"`

	// TargetCluster specifies where to deploy the chart.
	// Use "in-cluster" for the local cluster or a cluster URL for remote.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	TargetCluster string `json:"targetCluster"`

	// TargetNamespace specifies the namespace for chart resources.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	TargetNamespace string `json:"targetNamespace"`

	// Backend configures the deployment backend (Argo CD).
	// +kubebuilder:validation:Required
	Backend BackendSpec `json:"backend"`

	// ValuesSelectors defines static selectors for AddonValue resources.
	// Selected values are merged according to priority (lower first, higher overrides).
	// +optional
	ValuesSelectors []ValuesSelector `json:"valuesSelectors,omitempty"`

	// ValuesSources defines external sources for dynamic value extraction.
	// Values extracted from sources can be used in AddonValue templates.
	// +optional
	ValuesSources []ValueSource `json:"valuesSources,omitempty"`

	// Variables provides values for Go template rendering in AddonValue.
	// Accessible as .Variables.key in value templates.
	// +optional
	Variables map[string]string `json:"variables,omitempty"`

	// PluginName specifies an ArgoCD Config Management Plugin to use instead of
	// the built-in Helm source. When set, values are passed via HELM_VALUES
	// environment variable (base64-encoded) instead of source.helm.values.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	PluginName string `json:"pluginName,omitempty"`

	// ReleaseName overrides the Helm release name. In Helm mode, it maps to
	// source.helm.releaseName. In Plugin mode, it is passed as a RELEASE_NAME
	// environment variable.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// InitDependencies specifies dependencies that must be ready
	// before the Argo CD Application is created.
	// Useful for ordered deployment (e.g., cert-manager before its consumers).
	// +optional
	InitDependencies []Dependency `json:"initDependencies,omitempty"`
}

// AddonStatus defines the observed state of an Addon.
type AddonStatus struct {
	// ObservedGeneration is the last spec.generation processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// PhaseValuesSelector contains dynamic selectors injected by AddonPhase.
	// These are merged with spec.valuesSelectors according to priority.
	// Managed by AddonPhase controller, not user-editable.
	// +optional
	PhaseValuesSelector []ValuesSelector `json:"phaseValuesSelector,omitempty"`

	// ApplicationRef references the created Argo CD Application.
	// +optional
	ApplicationRef *ApplicationRef `json:"applicationRef,omitempty"`

	// ValuesHash is the hash of the final merged values.
	// Used to detect value changes for reconciliation.
	// +optional
	ValuesHash string `json:"valuesHash,omitempty"`

	// Deployed indicates that the Addon has been successfully deployed at least once.
	// Once set to true, this field is never reset to false.
	// +optional
	Deployed bool `json:"deployed,omitempty"`

	// Conditions represent the current state of the Addon.
	// Primary conditions: Ready, Progressing, Degraded.
	// Operational conditions: DependenciesMet, ValuesResolved, ApplicationCreated, Synced, Healthy.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Chart",type=string,JSONPath=`.spec.chart`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Deployed",type=boolean,JSONPath=`.status.deployed`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Addon is the primary resource for managing Helm-based deployments.
// It aggregates values from AddonValue resources, generates an Argo CD Application,
// and tracks deployment status. AddonPhase can dynamically inject additional
// value selectors based on cluster conditions.
type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the Addon.
	// +kubebuilder:validation:Required
	Spec AddonSpec `json:"spec"`

	// Status defines the observed state of the Addon.
	// +optional
	Status AddonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddonList contains a list of Addon resources.
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Addon{}, &AddonList{})
}
