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

// ValuesSelector defines how to select AddonValue resources.
// Values are selected by matching labels and merged according to priority.
// Lower priority values are applied first, higher priority values override.
type ValuesSelector struct {
	// Name identifies this selector for debugging and traceability.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Priority determines merge order. Lower values are applied first,
	// higher values override. Typical values: 0 (default), 50 (custom), 99 (immutable).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=0
	Priority int `json:"priority"`

	// MatchLabels selects AddonValue resources with matching labels.
	// All specified labels must match for selection.
	// +kubebuilder:validation:Required
	MatchLabels map[string]string `json:"matchLabels"`
}

// ValueSource defines an external source for extracting values.
// Values can be extracted from Kubernetes resources like Secrets or ConfigMaps.
type ValueSource struct {
	// Name identifies this source for debugging.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// SourceRef references the Kubernetes resource to extract from.
	// +kubebuilder:validation:Required
	SourceRef SourceRef `json:"sourceRef"`

	// Extract defines which values to extract and how.
	// +kubebuilder:validation:MinItems=1
	Extract []ExtractRule `json:"extract"`
}

// SourceRef references a Kubernetes resource.
type SourceRef struct {
	// APIVersion of the referenced resource (e.g., "v1").
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the referenced resource (e.g., "Secret", "ConfigMap").
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the referenced resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the referenced resource.
	// Required for namespaced resources, empty for cluster-scoped.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ExtractRule defines how to extract a value from a source resource.
type ExtractRule struct {
	// JSONPath specifies the path to extract from the resource.
	// Uses kubectl JSONPath syntax (e.g., ".data.key", ".data[\"ca.crt\"]").
	// +kubebuilder:validation:Required
	JSONPath string `json:"jsonPath"`

	// As specifies the target path in the merged values.
	// Uses dot notation (e.g., "tls.ca", "config.endpoint").
	// +kubebuilder:validation:Required
	As string `json:"as"`

	// Decode specifies optional decoding to apply.
	// Supported: "base64" (for Secret data).
	// +kubebuilder:validation:Enum=base64
	// +optional
	Decode string `json:"decode,omitempty"`
}

// Dependency specifies a blocking dependency that must be satisfied
// before the Addon can proceed with deployment.
type Dependency struct {
	// Name of the Addon to depend on.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Criteria specifies conditions that must be met.
	// All criteria must match for the dependency to be satisfied.
	// +kubebuilder:validation:MinItems=1
	Criteria []Criterion `json:"criteria"`
}

// Criterion defines a condition to evaluate against a resource.
// Used in both AddonPhase rules and Addon dependencies.
type Criterion struct {
	// Source specifies which resource to evaluate.
	// If omitted, evaluates against the dependency Addon.
	// +optional
	Source *CriterionSource `json:"source,omitempty"`

	// JSONPath specifies the path to the value in the resource.
	// Uses RFC 9535 JSONPath syntax (e.g., "$.status.phase",
	// "$.status.conditions[?@.type=='Ready'].status").
	// For keys with dots, use bracket notation: "$.metadata.labels['app.kubernetes.io/name']".
	// +kubebuilder:validation:Required
	JSONPath string `json:"jsonPath"`

	// Operator specifies the comparison operation.
	// +kubebuilder:validation:Required
	Operator CriterionOperator `json:"operator"`

	// Value to compare against. Required for comparison operators.
	// Use JSON encoding for non-string values.
	// +optional
	Value *apiextensionsv1.JSON `json:"value,omitempty"`

	// Keep controls whether this criterion participates in rule latching.
	// When true (default), once the rule matches, it stays matched permanently.
	// When false, the rule is re-evaluated every reconcile cycle.
	// +optional
	Keep *bool `json:"keep,omitempty"`
}

// CriterionSource identifies a Kubernetes resource to evaluate.
type CriterionSource struct {
	// APIVersion of the resource (e.g., "addons.in-cloud.io/v1alpha1").
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource (e.g., "Addon", "Secret").
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the resource.
	// Mutually exclusive with LabelSelector: exactly one must be specified.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace of the resource.
	// If empty, uses cluster-scoped lookup or same namespace as evaluating resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// LabelSelector for selecting multiple resources.
	// When set, all matching resources must satisfy the criterion.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// CriterionOperator defines supported comparison operators.
// +kubebuilder:validation:Enum=Equal;NotEqual;In;NotIn;Exists;NotExists;GreaterThan;GreaterOrEqual;LessThan;LessOrEqual;Matches
type CriterionOperator string

const (
	// OperatorEqual checks if values are equal.
	OperatorEqual CriterionOperator = "Equal"
	// OperatorNotEqual checks if values are not equal.
	OperatorNotEqual CriterionOperator = "NotEqual"
	// OperatorIn checks if value is in a list.
	OperatorIn CriterionOperator = "In"
	// OperatorNotIn checks if value is not in a list.
	OperatorNotIn CriterionOperator = "NotIn"
	// OperatorExists checks if path exists in the object.
	OperatorExists CriterionOperator = "Exists"
	// OperatorNotExists checks if path does not exist.
	OperatorNotExists CriterionOperator = "NotExists"
	// OperatorGreaterThan checks if value is greater than.
	OperatorGreaterThan CriterionOperator = "GreaterThan"
	// OperatorGreaterOrEqual checks if value is greater than or equal.
	OperatorGreaterOrEqual CriterionOperator = "GreaterOrEqual"
	// OperatorLessThan checks if value is less than.
	OperatorLessThan CriterionOperator = "LessThan"
	// OperatorLessOrEqual checks if value is less than or equal.
	OperatorLessOrEqual CriterionOperator = "LessOrEqual"
	// OperatorMatches checks if value matches a regex pattern.
	OperatorMatches CriterionOperator = "Matches"
)

// BackendSpec configures the deployment backend (e.g., Argo CD).
type BackendSpec struct {
	// Type specifies the backend type. Currently only "argocd" is supported.
	// +kubebuilder:validation:Enum=argocd
	// +kubebuilder:default=argocd
	Type string `json:"type"`

	// Namespace where the backend operates (e.g., "argocd").
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Project specifies the Argo CD project name.
	// +kubebuilder:default=default
	// +optional
	Project string `json:"project,omitempty"`

	// SyncPolicy configures automatic sync behavior.
	// +optional
	SyncPolicy *SyncPolicy `json:"syncPolicy,omitempty"`

	// IgnoreDifferences defines rules for ignoring differences during sync.
	// This is useful for fields that are managed by external controllers or mutating webhooks.
	// +optional
	IgnoreDifferences []ResourceIgnoreDifferences `json:"ignoreDifferences,omitempty"`
}

// ResourceIgnoreDifferences defines rules for ignoring differences for specific resources.
type ResourceIgnoreDifferences struct {
	// Group is the API group of the resource (e.g., "apps", "admissionregistration.k8s.io").
	// Empty string indicates the core API group.
	// +optional
	Group string `json:"group,omitempty"`

	// Kind is the resource kind (e.g., "Deployment", "ValidatingWebhookConfiguration").
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name is the resource name. If empty, applies to all resources of this kind.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the resource namespace. If empty, applies to all namespaces.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// JSONPointers is a list of JSON pointers to fields that should be ignored.
	// Uses RFC 6901 syntax (e.g., "/spec/replicas", "/metadata/annotations").
	// +optional
	JSONPointers []string `json:"jsonPointers,omitempty"`

	// JQPathExpressions is a list of JQ path expressions to fields that should be ignored.
	// +optional
	JQPathExpressions []string `json:"jqPathExpressions,omitempty"`

	// ManagedFieldsManagers is a list of managers that should be ignored when comparing fields.
	// +optional
	ManagedFieldsManagers []string `json:"managedFieldsManagers,omitempty"`
}

// SyncPolicy configures Argo CD automatic sync behavior.
type SyncPolicy struct {
	// Automated enables automatic sync when changes are detected.
	// +optional
	Automated *AutomatedSync `json:"automated,omitempty"`

	// SyncOptions provides additional sync options.
	// +optional
	SyncOptions []string `json:"syncOptions,omitempty"`

	// ManagedNamespaceMetadata controls metadata for the target namespace.
	// Labels and annotations specified here will be applied to the namespace
	// when CreateNamespace=true sync option is used.
	// +optional
	ManagedNamespaceMetadata *ManagedNamespaceMetadata `json:"managedNamespaceMetadata,omitempty"`
}

// ManagedNamespaceMetadata defines labels and annotations for target namespace.
type ManagedNamespaceMetadata struct {
	// Labels to apply to the target namespace.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to apply to the target namespace.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// AutomatedSync configures automatic synchronization.
type AutomatedSync struct {
	// Prune enables automatic pruning of resources no longer in Git.
	// +optional
	Prune bool `json:"prune,omitempty"`

	// SelfHeal enables automatic healing of out-of-sync resources.
	// +optional
	SelfHeal bool `json:"selfHeal,omitempty"`

	// AllowEmpty allows syncing when there are no resources.
	// +optional
	AllowEmpty bool `json:"allowEmpty,omitempty"`
}

// ApplicationRef references a created Argo CD Application.
type ApplicationRef struct {
	// Name of the Application resource.
	Name string `json:"name"`

	// Namespace where the Application is created.
	Namespace string `json:"namespace"`
}

// Note: Condition types and reasons have been moved to:
// - pkg/conditions/types.go (generic types like Ready, Progressing, Degraded)
// - internal/controller/conditions/types.go (Addon-specific types)
