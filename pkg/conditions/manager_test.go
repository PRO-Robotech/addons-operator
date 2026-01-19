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

func TestNew(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1)

	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.generation != 1 {
		t.Errorf("generation = %d, want 1", m.generation)
	}
}

func TestSetReady(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1)

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
	conds := []metav1.Condition{}
	m := New(&conds, 2)

	m.SetProgressing("NotSynced", "WaitingForSync", "Waiting for sync")

	// Check Ready=False with correct reason
	if !m.IsConditionFalse(TypeReady) {
		t.Error("Ready should be False")
	}
	readyCond := m.GetCondition(TypeReady)
	if readyCond.Reason != "NotSynced" {
		t.Errorf("Ready.Reason = %s, want 'NotSynced'", readyCond.Reason)
	}

	// Check Progressing=True
	if !m.IsConditionTrue(TypeProgressing) {
		t.Error("Progressing should be True")
	}
	progressingCond := m.GetCondition(TypeProgressing)
	if progressingCond.Reason != "WaitingForSync" {
		t.Errorf("Progressing.Reason = %s, want 'WaitingForSync'", progressingCond.Reason)
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
	conds := []metav1.Condition{}
	m := New(&conds, 3)

	m.SetDegraded("ValidationFailed", "ConfigError", "Configuration parsing failed")

	// Check Ready=False
	if !m.IsConditionFalse(TypeReady) {
		t.Error("Ready should be False")
	}
	readyCond := m.GetCondition(TypeReady)
	if readyCond.Reason != "ValidationFailed" {
		t.Errorf("Ready.Reason = %s, want 'ValidationFailed'", readyCond.Reason)
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
	if degradedCond.Reason != "ConfigError" {
		t.Errorf("Degraded.Reason = %s, want 'ConfigError'", degradedCond.Reason)
	}
	if degradedCond.Message != "Configuration parsing failed" {
		t.Errorf("Degraded.Message = %s, want 'Configuration parsing failed'", degradedCond.Message)
	}
}

func TestSetCondition(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1)

	// Set operational condition to true
	m.SetCondition("DatabaseReady", true, "Connected", "Connection established")

	if !m.IsConditionTrue("DatabaseReady") {
		t.Error("DatabaseReady should be True")
	}
	cond := m.GetCondition("DatabaseReady")
	if cond.Reason != "Connected" {
		t.Errorf("Reason = %s, want 'Connected'", cond.Reason)
	}

	// Set operational condition to false
	m.SetCondition("CacheWarmed", false, "Warming", "Loading entries")

	if !m.IsConditionFalse("CacheWarmed") {
		t.Error("CacheWarmed should be False")
	}
}

func TestEnsureAllConditions_EmptyStart(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1)

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
	conds := []metav1.Condition{
		{
			Type:   TypeReady,
			Status: metav1.ConditionTrue,
			Reason: ReasonFullyReconciled,
		},
	}
	m := New(&conds, 1)

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
	conds := []metav1.Condition{}
	fixedTime := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)

	m := New(&conds, 1, WithTimeFunc(func() time.Time { return fixedTime }))

	// Set initial state
	m.SetReady(ReasonFullyReconciled, "Initial")

	initialTransitionTime := m.GetCondition(TypeReady).LastTransitionTime

	// Advance time
	laterTime := fixedTime.Add(5 * time.Minute)
	m.now = func() time.Time { return laterTime }

	// Set same status again - LastTransitionTime should NOT change
	m.SetReady(ReasonFullyReconciled, "Same status")

	afterTransitionTime := m.GetCondition(TypeReady).LastTransitionTime

	if !initialTransitionTime.Equal(&afterTransitionTime) {
		t.Errorf("LastTransitionTime changed when status didn't change: %v -> %v",
			initialTransitionTime.Time, afterTransitionTime.Time)
	}

	// Change status - LastTransitionTime SHOULD change
	m.SetProgressing("NotSynced", "WaitingForSync", "Changed")

	changedTransitionTime := m.GetCondition(TypeReady).LastTransitionTime

	if initialTransitionTime.Equal(&changedTransitionTime) {
		t.Error("LastTransitionTime should change when status changes")
	}
}

func TestIsReady_IsProgressing_IsDegraded(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1)

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
	m.SetProgressing("NotSynced", "WaitingForSync", "")
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
	m.SetDegraded("ValidationFailed", "ConfigError", "")
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
	conds := []metav1.Condition{}
	m := New(&conds, 1)

	cond := m.GetCondition("NonExistent")
	if cond != nil {
		t.Error("GetCondition should return nil for non-existent condition")
	}
}

func TestObservedGeneration(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 42)

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
				m.SetProgressing("NotSynced", "WaitingForSync", "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionTrue,
			wantDegraded:    metav1.ConditionFalse,
		},
		{
			name: "Degraded state",
			transition: func(m *Manager) {
				m.SetDegraded("ValidationFailed", "ConfigError", "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionTrue,
		},
		{
			name: "Ready -> Progressing",
			transition: func(m *Manager) {
				m.SetReady(ReasonFullyReconciled, "")
				m.SetProgressing("NotSynced", "WaitingForSync", "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionTrue,
			wantDegraded:    metav1.ConditionFalse,
		},
		{
			name: "Progressing -> Degraded",
			transition: func(m *Manager) {
				m.SetProgressing("NotSynced", "WaitingForSync", "")
				m.SetDegraded("ValidationFailed", "ConfigError", "")
			},
			wantReady:       metav1.ConditionFalse,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionTrue,
		},
		{
			name: "Degraded -> Ready (recovery)",
			transition: func(m *Manager) {
				m.SetDegraded("ValidationFailed", "ConfigError", "")
				m.SetReady(ReasonFullyReconciled, "")
			},
			wantReady:       metav1.ConditionTrue,
			wantProgressing: metav1.ConditionFalse,
			wantDegraded:    metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conds := []metav1.Condition{}
			m := New(&conds, 1)

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

func TestConditions(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1)

	m.SetReady(ReasonFullyReconciled, "All done")

	result := m.Conditions()
	if len(result) != 3 {
		t.Errorf("Conditions() returned %d conditions, want 3", len(result))
	}
}

func TestWithTimeFunc(t *testing.T) {
	conds := []metav1.Condition{}
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)

	m := New(&conds, 1, WithTimeFunc(func() time.Time { return fixedTime }))

	m.SetReady(ReasonFullyReconciled, "")

	cond := m.GetCondition(TypeReady)
	if !cond.LastTransitionTime.Time.Equal(fixedTime) {
		t.Errorf("LastTransitionTime = %v, want %v", cond.LastTransitionTime.Time, fixedTime)
	}
}

func TestWithPrimaryConditions(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1, WithPrimaryConditions("Available", "Progressing", "Degraded"))

	m.EnsureAllConditions()

	// Should have "Available" instead of "Ready"
	if m.GetCondition("Available") == nil {
		t.Error("Available condition should exist")
	}
	// TypeReady ("Ready") should not exist because we customized primary conditions
	if m.GetCondition(TypeReady) != nil {
		t.Error("Ready condition should NOT exist when using custom primary conditions")
	}
	if m.GetCondition(TypeProgressing) == nil {
		t.Error("Progressing condition should exist")
	}
	if m.GetCondition(TypeDegraded) == nil {
		t.Error("Degraded condition should exist")
	}
}

func TestWithDefaultReasons(t *testing.T) {
	conds := []metav1.Condition{}
	m := New(&conds, 1, WithDefaultReasons(TypeReady, "ServiceReady", "ServiceNotReady"))

	m.EnsureAllConditions()

	// Ready should use custom default reason
	readyCond := m.GetCondition(TypeReady)
	if readyCond.Reason != "ServiceNotReady" {
		t.Errorf("Ready.Reason = %s, want 'ServiceNotReady'", readyCond.Reason)
	}
}

func TestMultipleOptions(t *testing.T) {
	conds := []metav1.Condition{}
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	m := New(&conds, 1,
		WithTimeFunc(func() time.Time { return fixedTime }),
		WithDefaultReasons(TypeReady, "CustomReady", "CustomNotReady"),
	)

	m.EnsureAllConditions()

	// Check time function works
	cond := m.GetCondition(TypeReady)
	if !cond.LastTransitionTime.Time.Equal(fixedTime) {
		t.Errorf("LastTransitionTime = %v, want %v", cond.LastTransitionTime.Time, fixedTime)
	}

	// Check custom reason works
	if cond.Reason != "CustomNotReady" {
		t.Errorf("Reason = %s, want 'CustomNotReady'", cond.Reason)
	}
}
