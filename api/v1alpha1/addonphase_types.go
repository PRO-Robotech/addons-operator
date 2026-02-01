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

// AddonPhaseSpec defines rules for dynamic value selector activation.
// Rules are evaluated against cluster state and inject selectors
// into the associated Addon's status when criteria are met.
type AddonPhaseSpec struct {
	// Rules defines conditions for activating additional value selectors.
	// Each rule independently evaluates criteria and injects its selector
	// into Addon.status.phaseValuesSelector when all criteria match.
	// +kubebuilder:validation:MinItems=1
	Rules []PhaseRule `json:"rules"`
}

// PhaseRule defines a single rule for conditional selector activation.
// When all criteria match, the selector is added to the target Addon's status.
type PhaseRule struct {
	// Name identifies this rule for debugging and status reporting.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Criteria defines conditions that must ALL be satisfied (AND logic).
	// If empty, the rule always matches (useful for unconditional activation).
	// +optional
	Criteria []Criterion `json:"criteria,omitempty"`

	// Selector defines the ValuesSelector to inject when criteria match.
	// This selector is added to Addon.status.phaseValuesSelector.
	// +kubebuilder:validation:Required
	Selector ValuesSelector `json:"selector"`
}

// AddonPhaseStatus defines the observed state of AddonPhase.
type AddonPhaseStatus struct {
	// ObservedGeneration is the last spec.generation processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// RuleStatuses reports the evaluation state of each rule.
	// +optional
	RuleStatuses []RuleStatus `json:"ruleStatuses,omitempty"`

	// Conditions represent the current state of the AddonPhase.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RuleStatus reports the evaluation state of a single rule.
type RuleStatus struct {
	// Name matches the rule name from spec.rules[].name.
	Name string `json:"name"`

	// Matched indicates whether all criteria are currently satisfied.
	Matched bool `json:"matched"`

	// Latched indicates that keep=true criteria in this rule have previously
	// matched and will be skipped on subsequent evaluations.
	// Only keep=false criteria continue to be re-evaluated.
	// +optional
	Latched bool `json:"latched,omitempty"`

	// Message provides additional context about the evaluation.
	// +optional
	Message string `json:"message,omitempty"`

	// LastEvaluated is the timestamp of the last evaluation.
	// +optional
	LastEvaluated metav1.Time `json:"lastEvaluated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AddonPhase is a rule engine for conditional value selector activation.
// It evaluates criteria against cluster state and injects matching selectors
// into the associated Addon's status.phaseValuesSelector.
//
// Relationship: AddonPhase has a 1:1 relationship with Addon by name.
// The AddonPhase named "foo" manages selectors for the Addon named "foo".
//
// Workflow:
//  1. AddonPhase evaluates each rule's criteria against cluster resources
//  2. For matched rules, the selector is added to Addon.status.phaseValuesSelector
//  3. Addon controller merges phaseValuesSelector with spec.valuesSelectors
//  4. Final values are aggregated and applied to the Argo CD Application
type AddonPhase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the rules for selector activation.
	// +kubebuilder:validation:Required
	Spec AddonPhaseSpec `json:"spec"`

	// Status defines the observed state.
	// +optional
	Status AddonPhaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddonPhaseList contains a list of AddonPhase resources.
type AddonPhaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonPhase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonPhase{}, &AddonPhaseList{})
}
