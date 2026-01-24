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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewManager(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 1)

	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	// Verify manager works by calling a method
	m.EnsureAllConditions()
	if len(conditions) != 3 {
		t.Errorf("Expected 3 conditions after EnsureAllConditions, got %d", len(conditions))
	}
}

func TestSetReady(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 1)

	m.SetReady(ReasonFullyReconciled, "All done")

	// Check Ready=True
	if !m.IsConditionTrue(TypeReady) {
		t.Error("Ready should be True")
	}
	readyCond := m.GetCondition(TypeReady)
	if readyCond.Reason != ReasonFullyReconciled {
		t.Errorf("Ready.Reason = %s, want %s", readyCond.Reason, ReasonFullyReconciled)
	}
	if readyCond.Message != "All done" {
		t.Errorf("Ready.Message = %s, want 'All done'", readyCond.Message)
	}

	// Check Progressing=False
	if !m.IsConditionFalse(TypeProgressing) {
		t.Error("Progressing should be False")
	}
	progressingCond := m.GetCondition(TypeProgressing)
	if progressingCond.Reason != ReasonComplete {
		t.Errorf("Progressing.Reason = %s, want %s", progressingCond.Reason, ReasonComplete)
	}

	// Check Degraded=False
	if !m.IsConditionFalse(TypeDegraded) {
		t.Error("Degraded should be False")
	}
	degradedCond := m.GetCondition(TypeDegraded)
	if degradedCond.Reason != ReasonHealthy {
		t.Errorf("Degraded.Reason = %s, want %s", degradedCond.Reason, ReasonHealthy)
	}
}

func TestSetProgressing(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 2)

	m.SetProgressing(ReasonNotSynced, ReasonWaitingForSync, "Waiting for ArgoCD sync")

	// Check Ready=False with correct reason
	if !m.IsConditionFalse(TypeReady) {
		t.Error("Ready should be False")
	}
	readyCond := m.GetCondition(TypeReady)
	if readyCond.Reason != ReasonNotSynced {
		t.Errorf("Ready.Reason = %s, want %s", readyCond.Reason, ReasonNotSynced)
	}

	// Check Progressing=True
	if !m.IsConditionTrue(TypeProgressing) {
		t.Error("Progressing should be True")
	}
	progressingCond := m.GetCondition(TypeProgressing)
	if progressingCond.Reason != ReasonWaitingForSync {
		t.Errorf("Progressing.Reason = %s, want %s", progressingCond.Reason, ReasonWaitingForSync)
	}

	// Check Degraded=False
	if !m.IsConditionFalse(TypeDegraded) {
		t.Error("Degraded should be False")
	}

	// Check ObservedGeneration
	if readyCond.ObservedGeneration != 2 {
		t.Errorf("Ready.ObservedGeneration = %d, want 2", readyCond.ObservedGeneration)
	}
}

func TestSetDegraded(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 3)

	m.SetDegraded(ReasonValuesNotResolved, ReasonTemplateError, "Template parsing failed")

	// Check Ready=False
	if !m.IsConditionFalse(TypeReady) {
		t.Error("Ready should be False")
	}
	readyCond := m.GetCondition(TypeReady)
	if readyCond.Reason != ReasonValuesNotResolved {
		t.Errorf("Ready.Reason = %s, want %s", readyCond.Reason, ReasonValuesNotResolved)
	}

	// Check Progressing=False
	if !m.IsConditionFalse(TypeProgressing) {
		t.Error("Progressing should be False")
	}

	// Check Degraded=True
	if !m.IsConditionTrue(TypeDegraded) {
		t.Error("Degraded should be True")
	}
	degradedCond := m.GetCondition(TypeDegraded)
	if degradedCond.Reason != ReasonTemplateError {
		t.Errorf("Degraded.Reason = %s, want %s", degradedCond.Reason, ReasonTemplateError)
	}
	if degradedCond.Message != "Template parsing failed" {
		t.Errorf("Degraded.Message = %s, want 'Template parsing failed'", degradedCond.Message)
	}
}

func TestSetOperationalCondition(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 1)

	// Set operational condition to true
	m.SetOperationalCondition(TypeDependenciesMet, true, "AllSatisfied", "All dependencies ready")

	if !m.IsConditionTrue(TypeDependenciesMet) {
		t.Error("DependenciesMet should be True")
	}
	cond := m.GetCondition(TypeDependenciesMet)
	if cond.Reason != "AllSatisfied" {
		t.Errorf("Reason = %s, want 'AllSatisfied'", cond.Reason)
	}

	// Set operational condition to false
	m.SetOperationalCondition(TypeSynced, false, "OutOfSync", "Application out of sync")

	if !m.IsConditionFalse(TypeSynced) {
		t.Error("Synced should be False")
	}
}

func TestEnsureAllConditions_EmptyStart(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 1)

	m.EnsureAllConditions()

	// Should initialize to Progressing state
	if !m.IsConditionFalse(TypeReady) {
		t.Error("Ready should be False initially")
	}
	if !m.IsConditionTrue(TypeProgressing) {
		t.Error("Progressing should be True initially")
	}
	if !m.IsConditionFalse(TypeDegraded) {
		t.Error("Degraded should be False initially")
	}
}

func TestEnsureAllConditions_MissingCondition(t *testing.T) {
	// Start with only Ready condition
	conditions := []metav1.Condition{
		{
			Type:   TypeReady,
			Status: metav1.ConditionTrue,
			Reason: ReasonFullyReconciled,
		},
	}
	m := NewManager(&conditions, 1)

	m.EnsureAllConditions()

	// All three primary conditions should exist
	if m.GetCondition(TypeReady) == nil {
		t.Error("Ready condition should exist")
	}
	if m.GetCondition(TypeProgressing) == nil {
		t.Error("Progressing condition should exist")
	}
	if m.GetCondition(TypeDegraded) == nil {
		t.Error("Degraded condition should exist")
	}
}

func TestLastTransitionTimePreserved(t *testing.T) {
	conditions := []metav1.Condition{}
	fixedTime := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)

	m := NewManagerWithTimeFunc(&conditions, 1, func() time.Time { return fixedTime })

	// Set initial state
	m.SetReady(ReasonFullyReconciled, "Initial")

	initialTransitionTime := m.GetCondition(TypeReady).LastTransitionTime

	// Advance time - need to create new manager with new time func
	laterTime := fixedTime.Add(5 * time.Minute)
	m2 := NewManagerWithTimeFunc(&conditions, 1, func() time.Time { return laterTime })

	// Set same status again - LastTransitionTime should NOT change
	m2.SetReady(ReasonFullyReconciled, "Same status")

	afterTransitionTime := m2.GetCondition(TypeReady).LastTransitionTime

	if !initialTransitionTime.Equal(&afterTransitionTime) {
		t.Errorf("LastTransitionTime changed when status didn't change: %v -> %v",
			initialTransitionTime.Time, afterTransitionTime.Time)
	}

	// Change status - LastTransitionTime SHOULD change
	m2.SetProgressing(ReasonNotSynced, ReasonWaitingForSync, "Changed")

	changedTransitionTime := m2.GetCondition(TypeReady).LastTransitionTime

	if initialTransitionTime.Equal(&changedTransitionTime) {
		t.Error("LastTransitionTime should change when status changes")
	}
}

func TestIsReady_IsProgressing_IsDegraded(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 1)

	// Initially nothing set
	if m.IsReady() {
		t.Error("IsReady should be false initially")
	}
	if m.IsProgressing() {
		t.Error("IsProgressing should be false initially")
	}
	if m.IsDegraded() {
		t.Error("IsDegraded should be false initially")
	}

	// Set ready
	m.SetReady(ReasonFullyReconciled, "")
	if !m.IsReady() {
		t.Error("IsReady should be true after SetReady")
	}
	if m.IsProgressing() {
		t.Error("IsProgressing should be false after SetReady")
	}
	if m.IsDegraded() {
		t.Error("IsDegraded should be false after SetReady")
	}

	// Set progressing
	m.SetProgressing(ReasonNotSynced, ReasonWaitingForSync, "")
	if m.IsReady() {
		t.Error("IsReady should be false after SetProgressing")
	}
	if !m.IsProgressing() {
		t.Error("IsProgressing should be true after SetProgressing")
	}
	if m.IsDegraded() {
		t.Error("IsDegraded should be false after SetProgressing")
	}

	// Set degraded
	m.SetDegraded(ReasonValuesNotResolved, ReasonTemplateError, "")
	if m.IsReady() {
		t.Error("IsReady should be false after SetDegraded")
	}
	if m.IsProgressing() {
		t.Error("IsProgressing should be false after SetDegraded")
	}
	if !m.IsDegraded() {
		t.Error("IsDegraded should be true after SetDegraded")
	}
}

func TestGetCondition_NotFound(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 1)

	cond := m.GetCondition("NonExistent")
	if cond != nil {
		t.Error("GetCondition should return nil for non-existent condition")
	}
}

func TestObservedGeneration(t *testing.T) {
	conditions := []metav1.Condition{}
	m := NewManager(&conditions, 42)

	m.SetReady(ReasonFullyReconciled, "")

	readyCond := m.GetCondition(TypeReady)
	if readyCond.ObservedGeneration != 42 {
		t.Errorf("ObservedGeneration = %d, want 42", readyCond.ObservedGeneration)
	}

	progressingCond := m.GetCondition(TypeProgressing)
	if progressingCond.ObservedGeneration != 42 {
		t.Errorf("Progressing.ObservedGeneration = %d, want 42", progressingCond.ObservedGeneration)
	}

	degradedCond := m.GetCondition(TypeDegraded)
	if degradedCond.ObservedGeneration != 42 {
		t.Errorf("Degraded.ObservedGeneration = %d, want 42", degradedCond.ObservedGeneration)
	}
}

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name            string
		transition      func(m *Manager)
		wantReady       metav1.ConditionStatus
		wantProgressing metav1.ConditionStatus
		wantDegraded    metav1.ConditionStatus
	}{
		{
			name: "Ready state",
			transition: func(m *Manager) {
				m.SetReady(ReasonFullyReconciled, "")
			},
			wantReady:       metav1.ConditionTrue,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionFalse,
		},
		{
			name: "Progressing state",
			transition: func(m *Manager) {
				m.SetProgressing(ReasonNotSynced, ReasonWaitingForSync, "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionTrue,
			wantDegraded:    metav1.ConditionFalse,
		},
		{
			name: "Degraded state",
			transition: func(m *Manager) {
				m.SetDegraded(ReasonValuesNotResolved, ReasonTemplateError, "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionTrue,
		},
		{
			name: "Ready -> Progressing",
			transition: func(m *Manager) {
				m.SetReady(ReasonFullyReconciled, "")
				m.SetProgressing(ReasonNotSynced, ReasonWaitingForSync, "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionTrue,
			wantDegraded:    metav1.ConditionFalse,
		},
		{
			name: "Progressing -> Degraded",
			transition: func(m *Manager) {
				m.SetProgressing(ReasonNotSynced, ReasonWaitingForSync, "")
				m.SetDegraded(ReasonValuesNotResolved, ReasonTemplateError, "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionTrue,
		},
		{
			name: "Degraded -> Ready (recovery)",
			transition: func(m *Manager) {
				m.SetDegraded(ReasonValuesNotResolved, ReasonTemplateError, "")
				m.SetReady(ReasonFullyReconciled, "")
			},
			wantReady:       metav1.ConditionTrue,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := []metav1.Condition{}
			m := NewManager(&conditions, 1)

			tt.transition(m)

			readyCond := m.GetCondition(TypeReady)
			if readyCond.Status != tt.wantReady {
				t.Errorf("Ready.Status = %s, want %s", readyCond.Status, tt.wantReady)
			}

			progressingCond := m.GetCondition(TypeProgressing)
			if progressingCond.Status != tt.wantProgressing {
				t.Errorf("Progressing.Status = %s, want %s", progressingCond.Status, tt.wantProgressing)
			}

			degradedCond := m.GetCondition(TypeDegraded)
			if degradedCond.Status != tt.wantDegraded {
				t.Errorf("Degraded.Status = %s, want %s", degradedCond.Status, tt.wantDegraded)
			}
		})
	}
}

func TestNewManagerWithTimeFunc(t *testing.T) {
	conditions := []metav1.Condition{}
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)

	m := NewManagerWithTimeFunc(&conditions, 1, func() time.Time { return fixedTime })

	m.SetReady(ReasonFullyReconciled, "")

	cond := m.GetCondition(TypeReady)
	if !cond.LastTransitionTime.Time.Equal(fixedTime) {
		t.Errorf("LastTransitionTime = %v, want %v", cond.LastTransitionTime.Time, fixedTime)
	}
}
