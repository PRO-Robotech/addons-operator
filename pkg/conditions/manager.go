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

package conditions

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultReasons holds default reason strings for a condition type.
type defaultReasons struct {
	whenTrue  string
	whenFalse string
}

// Manager handles condition updates with proper transition semantics.
// It provides a state machine abstraction over Kubernetes conditions with guarantees:
//   - All primary conditions always present with explicit True/False status
//   - LastTransitionTime only changes on actual status change
//   - Consistent Reason/Message formatting
//   - ObservedGeneration tracking on all conditions
type Manager struct {
	conditions *[]metav1.Condition
	generation int64
	now        func() time.Time

	primaryConditions []string
	defaultReasons    map[string]defaultReasons
}

// New creates a new Manager for managing conditions on a Kubernetes resource.
// The conditions parameter should be a pointer to the Status.Conditions slice.
// Generation should be the resource's metadata.generation for ObservedGeneration tracking.
//
// Example:
//
//	cm := conditions.New(&myResource.Status.Conditions, myResource.Generation)
//	cm.EnsureAllConditions()
//	// ... reconciliation logic ...
//	cm.SetReady(conditions.ReasonFullyReconciled, "All systems operational")
func New(conditions *[]metav1.Condition, generation int64, opts ...Option) *Manager {
	m := &Manager{
		conditions:        conditions,
		generation:        generation,
		now:               time.Now,
		primaryConditions: []string{TypeReady, TypeProgressing, TypeDegraded},
		defaultReasons: map[string]defaultReasons{
			TypeReady:       {whenTrue: ReasonFullyReconciled, whenFalse: ReasonInitializing},
			TypeProgressing: {whenTrue: ReasonReconciling, whenFalse: ReasonComplete},
			TypeDegraded:    {whenTrue: ReasonError, whenFalse: ReasonHealthy},
		},
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// SetReady marks the resource as ready (successful reconciliation).
// This sets: Ready=True, Progressing=False, Degraded=False
//
// Use this when reconciliation has completed successfully and the resource
// is operating normally.
func (m *Manager) SetReady(reason, message string) {
	m.set(TypeReady, metav1.ConditionTrue, reason, message)
	m.set(TypeProgressing, metav1.ConditionFalse, ReasonComplete, "")
	m.set(TypeDegraded, metav1.ConditionFalse, ReasonHealthy, "")
}

// SetProgressing marks the resource as progressing (reconciliation in progress).
// This sets: Ready=False, Progressing=True, Degraded=False
//
// Parameters:
//   - readyReason: explains why Ready is false (e.g., "NotSynced", "WaitingForDependency")
//   - progressingReason: explains what is in progress (e.g., "Reconciling", "WaitingForSync")
//   - message: human-readable description of current progress
//
// Use this during active reconciliation when the resource is not yet ready
// but no errors have occurred.
func (m *Manager) SetProgressing(readyReason, progressingReason, message string) {
	m.set(TypeReady, metav1.ConditionFalse, readyReason, message)
	m.set(TypeProgressing, metav1.ConditionTrue, progressingReason, message)
	m.set(TypeDegraded, metav1.ConditionFalse, ReasonHealthy, "")
}

// SetDegraded marks the resource as degraded (error state).
// This sets: Ready=False, Progressing=False, Degraded=True
//
// Parameters:
//   - readyReason: explains why Ready is false (e.g., "ValidationFailed", "DependencyError")
//   - degradedReason: explains the error type (e.g., "ConfigError", "BackendUnavailable")
//   - message: human-readable error description
//
// Use this when an error has occurred that prevents successful reconciliation.
// The resource will typically be requeued with a longer backoff.
func (m *Manager) SetDegraded(readyReason, degradedReason, message string) {
	m.set(TypeReady, metav1.ConditionFalse, readyReason, message)
	m.set(TypeProgressing, metav1.ConditionFalse, ReasonComplete, "")
	m.set(TypeDegraded, metav1.ConditionTrue, degradedReason, message)
}

// SetCondition sets a condition with the given parameters.
// Use this for operational/observability conditions beyond the primary state machine.
//
// Example:
//
//	cm.SetCondition("DatabaseReady", true, "Connected", "Connection established")
//	cm.SetCondition("CacheWarmed", false, "Warming", "50% complete")
func (m *Manager) SetCondition(condType string, status bool, reason, message string) {
	condStatus := metav1.ConditionFalse
	if status {
		condStatus = metav1.ConditionTrue
	}
	m.set(condType, condStatus, reason, message)
}

// IsConditionTrue returns true if the condition exists and has Status=True.
func (m *Manager) IsConditionTrue(condType string) bool {
	cond := meta.FindStatusCondition(*m.conditions, condType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsConditionFalse returns true if the condition exists and has Status=False.
func (m *Manager) IsConditionFalse(condType string) bool {
	cond := meta.FindStatusCondition(*m.conditions, condType)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

// GetCondition returns the condition with the given type, or nil if not found.
func (m *Manager) GetCondition(condType string) *metav1.Condition {
	return meta.FindStatusCondition(*m.conditions, condType)
}

// IsReady returns true if Ready=True.
func (m *Manager) IsReady() bool {
	return m.IsConditionTrue(TypeReady)
}

// IsProgressing returns true if Progressing=True.
func (m *Manager) IsProgressing() bool {
	return m.IsConditionTrue(TypeProgressing)
}

// IsDegraded returns true if Degraded=True.
func (m *Manager) IsDegraded() bool {
	return m.IsConditionTrue(TypeDegraded)
}

// EnsureAllConditions ensures all primary conditions exist with explicit status.
// Should be called at the start of each reconcile loop.
// Missing conditions are initialized to their default state (Progressing).
//
// This method guarantees that after calling, all primary conditions
// (Ready, Progressing, Degraded by default) will exist with explicit
// True/False status, never Unknown.
func (m *Manager) EnsureAllConditions() {
	// Ensure each primary condition exists
	for _, condType := range m.primaryConditions {
		if meta.FindStatusCondition(*m.conditions, condType) == nil {
			reasons, ok := m.defaultReasons[condType]
			if !ok {
				reasons = defaultReasons{whenTrue: "True", whenFalse: "False"}
			}

			// Initialize based on condition type
			// First primary condition (typically Ready-like) starts False
			// Second primary condition (typically Progressing-like) starts True
			// Third primary condition (typically Degraded-like) starts False
			switch condType {
			case TypeReady:
				m.set(condType, metav1.ConditionFalse, reasons.whenFalse, "Condition not yet evaluated")
			case TypeProgressing:
				m.set(condType, metav1.ConditionTrue, reasons.whenTrue, "Reconciliation in progress")
			case TypeDegraded:
				m.set(condType, metav1.ConditionFalse, reasons.whenFalse, "")
			default:
				// For custom primary conditions, determine initial state by position
				// This allows custom state machines with different condition names
				switch m.indexOfPrimaryCondition(condType) {
				case 0:
					// First condition (Ready-equivalent): False initially
					m.set(condType, metav1.ConditionFalse, reasons.whenFalse, "Condition not yet evaluated")
				case 1:
					// Second condition (Progressing-equivalent): True initially
					m.set(condType, metav1.ConditionTrue, reasons.whenTrue, "Reconciliation in progress")
				default:
					// Third+ conditions (Degraded-equivalent): False initially
					m.set(condType, metav1.ConditionFalse, reasons.whenFalse, "")
				}
			}
		}
	}
}

// indexOfPrimaryCondition returns the index of condType in primaryConditions, or -1 if not found.
func (m *Manager) indexOfPrimaryCondition(condType string) int {
	for i, ct := range m.primaryConditions {
		if ct == condType {
			return i
		}
	}
	return -1
}

// set updates or creates a condition with proper LastTransitionTime handling.
// LastTransitionTime only changes when status actually changes (handled by meta.SetStatusCondition).
func (m *Manager) set(condType string, status metav1.ConditionStatus, reason, message string) {
	newCondition := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: m.generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(m.now()),
	}

	// meta.SetStatusCondition handles LastTransitionTime preservation
	// when status doesn't change
	meta.SetStatusCondition(m.conditions, newCondition)
}

// Conditions returns the underlying conditions slice.
// Use this when you need direct access to the conditions for serialization.
func (m *Manager) Conditions() []metav1.Condition {
	return *m.conditions
}
