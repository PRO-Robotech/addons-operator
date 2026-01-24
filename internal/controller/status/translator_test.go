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

package status

import (
	"testing"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"addons-operator/internal/controller/conditions"
)

// newTestConditionManager creates a condition manager for testing.
func newTestConditionManager(conds *[]metav1.Condition, generation int64) *conditions.Manager {
	return conditions.NewManager(conds, generation)
}

func TestStatusTranslator_Translate_SyncedAndHealthy(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusHealthy,
			},
		},
	}

	conds := translator.Translate(app, 1)

	require.Len(t, conds, 4)

	// Check ApplicationCreated
	appCreated := findCondition(conds, conditions.TypeApplicationCreated)
	require.NotNil(t, appCreated)
	assert.Equal(t, metav1.ConditionTrue, appCreated.Status)
	assert.Equal(t, "ApplicationExists", appCreated.Reason)

	// Check Synced
	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionTrue, synced.Status)
	assert.Equal(t, "Synced", synced.Reason)

	// Check Healthy
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionTrue, healthy.Status)
	assert.Equal(t, "Healthy", healthy.Reason)

	// Check Ready (should be True since both Synced and Healthy are True)
	ready := findCondition(conds, conditions.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
	assert.Equal(t, "Ready", ready.Reason)
}

func TestStatusTranslator_Translate_OutOfSync(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeOutOfSync,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusHealthy,
			},
		},
	}

	conds := translator.Translate(app, 1)

	// Check Synced is False
	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, "OutOfSync", synced.Reason)

	// Check Ready is False (because Synced is False)
	ready := findCondition(conds, conditions.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "NotSynced", ready.Reason)
}

func TestStatusTranslator_Translate_HealthDegraded(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status:  health.HealthStatusDegraded,
				Message: "Pod is crashing",
			},
		},
	}

	conds := translator.Translate(app, 1)

	// Check Healthy is False with Degraded reason
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "Degraded", healthy.Reason)
	assert.Equal(t, "Pod is crashing", healthy.Message)

	// Check Ready is False (because Healthy is False)
	ready := findCondition(conds, conditions.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "Degraded", ready.Reason)
}

func TestStatusTranslator_Translate_HealthProgressing(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusProgressing,
			},
		},
	}

	conds := translator.Translate(app, 1)

	// Check Healthy is False with Progressing reason
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "Progressing", healthy.Reason)

	// Check Ready is False
	ready := findCondition(conds, conditions.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "Progressing", ready.Reason)
}

func TestStatusTranslator_Translate_HealthSuspended(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusSuspended,
			},
		},
	}

	conds := translator.Translate(app, 1)

	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "Suspended", healthy.Reason)
}

func TestStatusTranslator_Translate_HealthMissing(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusMissing,
			},
		},
	}

	conds := translator.Translate(app, 1)

	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "Missing", healthy.Reason)
}

func TestStatusTranslator_Translate_UnknownStatuses(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "argocd",
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeUnknown,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusUnknown,
			},
		},
	}

	conds := translator.Translate(app, 1)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionUnknown, synced.Status)
	assert.Equal(t, "Unknown", synced.Reason)

	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionUnknown, healthy.Status)
	assert.Equal(t, "Unknown", healthy.Reason)
}

func TestStatusTranslator_UpdateConditions_SyncedAndHealthy(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusHealthy,
			},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds, 1)
	translator.UpdateConditions(cm, app)

	// Check Synced condition is True
	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionTrue, synced.Status)
	assert.Equal(t, "Synced", synced.Reason)

	// Check Healthy condition is True
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionTrue, healthy.Status)
	assert.Equal(t, "Healthy", healthy.Reason)
}

func TestStatusTranslator_UpdateConditions_OutOfSyncDegraded(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeOutOfSync,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status:  health.HealthStatusDegraded,
				Message: "Pod is crashing",
			},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds, 1)
	translator.UpdateConditions(cm, app)

	// Check Synced condition is False
	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, "OutOfSync", synced.Reason)

	// Check Healthy condition is False
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "Degraded", healthy.Reason)
	assert.Equal(t, "Pod is crashing", healthy.Message)
}

func TestSetCondition_PreservesTransitionTime(t *testing.T) {
	oldTime := metav1.Now()

	conds := []metav1.Condition{
		{
			Type:               "Test",
			Status:             metav1.ConditionTrue,
			Reason:             "OldReason",
			Message:            "Old message",
			LastTransitionTime: oldTime,
		},
	}

	// Update with same status - transition time should be preserved
	newCondition := metav1.Condition{
		Type:    "Test",
		Status:  metav1.ConditionTrue,
		Reason:  "NewReason",
		Message: "New message",
	}

	SetCondition(&conds, newCondition)

	require.Len(t, conds, 1)
	assert.Equal(t, "NewReason", conds[0].Reason)
	assert.Equal(t, "New message", conds[0].Message)
	assert.Equal(t, oldTime, conds[0].LastTransitionTime)
}

func TestSetCondition_UpdatesTransitionTimeOnStatusChange(t *testing.T) {
	oldTime := metav1.Now()

	conds := []metav1.Condition{
		{
			Type:               "Test",
			Status:             metav1.ConditionTrue,
			Reason:             "OldReason",
			LastTransitionTime: oldTime,
		},
	}

	// Update with different status - transition time should change
	newCondition := metav1.Condition{
		Type:   "Test",
		Status: metav1.ConditionFalse,
		Reason: "NewReason",
	}

	SetCondition(&conds, newCondition)

	require.Len(t, conds, 1)
	assert.Equal(t, metav1.ConditionFalse, conds[0].Status)
	// Transition time should be newer or equal (test might run fast)
	assert.True(t, !conds[0].LastTransitionTime.Before(&oldTime))
}

func TestSetCondition_AppendsNewCondition(t *testing.T) {
	conds := []metav1.Condition{}

	newCondition := metav1.Condition{
		Type:   "Test",
		Status: metav1.ConditionTrue,
		Reason: "TestReason",
	}

	SetCondition(&conds, newCondition)

	require.Len(t, conds, 1)
	assert.Equal(t, "Test", conds[0].Type)
	assert.Equal(t, metav1.ConditionTrue, conds[0].Status)
}

// Helper function
func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}
